// Command qorm-donors refreshes the public supporters list from Patreon.
//
// It reads the campaign's patrons via the Patreon API v2, formats a
// privacy-friendly name + lifetime support, and writes the donors.json the
// website renders. Run daily (see deploy/donors). Credentials come from the
// environment and are NEVER committed:
//
//	PATREON_ACCESS_TOKEN   a Creator's Access Token (Patreon developer portal)
//	PATREON_CAMPAIGN_ID    optional; auto-discovered from the token if unset
//
//	qorm-donors [-o donors.json]
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

const apiBase = "https://www.patreon.com/api/oauth2/v2"

var httpClient = &http.Client{Timeout: 30 * time.Second}

type donor struct {
	Name     string  `json:"name"`
	Total    float64 `json:"total"`
	Currency string  `json:"currency"`
}

type output struct {
	Updated string  `json:"updated"`
	Count   int     `json:"count"`
	Total   float64 `json:"total"`
	Donors  []donor `json:"donors"`
}

// get does an authenticated GET and decodes the JSON:API body into v.
func get(token, url string, v any) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if resp.StatusCode != 200 {
		return fmt.Errorf("patreon %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return json.Unmarshal(body, v)
}

// campaignID returns the configured campaign, or the token owner's first one.
func campaignID(token string) (string, error) {
	if id := os.Getenv("PATREON_CAMPAIGN_ID"); id != "" {
		return id, nil
	}
	var r struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := get(token, apiBase+"/campaigns", &r); err != nil {
		return "", err
	}
	if len(r.Data) == 0 {
		return "", fmt.Errorf("no Patreon campaign found for this token")
	}
	return r.Data[0].ID, nil
}

type memberPage struct {
	Data []struct {
		Attributes struct {
			FullName      string `json:"full_name"`
			LifetimeCents int    `json:"lifetime_support_cents"`
			Status        string `json:"patron_status"`
		} `json:"attributes"`
	} `json:"data"`
	Links struct {
		Next string `json:"next"`
	} `json:"links"`
}

// displayName renders a privacy-friendly name: "Ada L." (first + last initial).
func displayName(full string) string {
	full = strings.TrimSpace(full)
	if full == "" {
		return "Anonymous"
	}
	parts := strings.Fields(full)
	name := parts[0]
	if len(parts) > 1 {
		if r := []rune(parts[len(parts)-1]); len(r) > 0 {
			name += " " + string(r[0]) + "."
		}
	}
	return name
}

func main() {
	out := flag.String("o", "donors.json", "output path")
	flag.Parse()

	token := os.Getenv("PATREON_ACCESS_TOKEN")
	if token == "" {
		fmt.Fprintln(os.Stderr, "error: set PATREON_ACCESS_TOKEN (a Creator's Access Token from the Patreon developer portal).")
		fmt.Fprintln(os.Stderr, "       existing donors.json left untouched.")
		os.Exit(1)
	}
	cid, err := campaignID(token)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	res := output{Updated: time.Now().UTC().Format(time.RFC3339)}
	url := apiBase + "/campaigns/" + cid + "/members?fields%5Bmember%5D=full_name,lifetime_support_cents,patron_status&page%5Bsize%5D=1000"
	seen := map[string]bool{}
	for url != "" {
		if seen[url] { // cyclic links.next — stop rather than loop forever
			break
		}
		seen[url] = true
		var p memberPage
		if err := get(token, url, &p); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		for _, m := range p.Data {
			if m.Attributes.LifetimeCents <= 0 {
				continue // only people who've actually supported
			}
			res.Donors = append(res.Donors, donor{
				Name:     displayName(m.Attributes.FullName),
				Total:    float64(m.Attributes.LifetimeCents) / 100,
				Currency: "USD",
			})
		}
		url = p.Links.Next
	}

	for _, d := range res.Donors {
		res.Total += d.Total
	}
	sort.Slice(res.Donors, func(i, j int) bool { return res.Donors[i].Total > res.Donors[j].Total })
	res.Count = len(res.Donors)

	// Never blank a good list: 0 supporters almost always means an API/schema
	// hiccup (e.g. a renamed field), not that everyone unsubscribed. Leave the
	// existing donors.json in place rather than overwrite it with an empty one.
	if res.Count == 0 {
		fmt.Fprintln(os.Stderr, "error: 0 supporters returned — leaving the existing donors.json untouched (guarding against API drift).")
		os.Exit(1)
	}

	b, _ := json.MarshalIndent(res, "", "  ")
	if err := os.WriteFile(*out, b, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "wrote %s — %d supporters, %.2f total\n", *out, res.Count, res.Total)
}
