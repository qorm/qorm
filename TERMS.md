# QORM Terms

QORM's **source code** is MIT-licensed (see [LICENSE](LICENSE)) — free to use,
modify, and distribute. These Terms add a good-faith **branding condition** for
commercial white-labeling; they don't restrict your MIT rights to the code.

## Tiers

| Tier | Price | For |
|---|---|---|
| **Community** | Free | personal / educational / open-source use |
| **Indie** | US$1 / month | individual commercial white-labeling |
| **Studio** | US$7 / month | company commercial white-labeling |
| **Supporter** | US$3 / month | backing QORM — no commercial rights, but perks below |

> **Patreon:** https://www.patreon.com/qorm

## Community — free

Personal, educational, and open-source use is **entirely free**. Apps ship with
the QORM logo as the default icon and a "Made with QORM" note in their packaging
metadata (not shown in the UI).

## Secondary development (forks, libraries, tools)

Building **on** QORM's open source — forking it, extending the runtime, or
shipping a library/tool that uses it — is free under the MIT License. The only
ask is **attribution**: state that your work uses / is based on QORM (and keep the
MIT copyright notice, which the license already requires). No membership needed.

This is different from shipping a **product app** you built with QORM and
white-labeling it commercially — that's the Indie / Studio tiers below.

## Indie / Studio — commercial white-labeling

If you **sell or monetize** a product and want to **white-label** it — replace the
QORM icon with your own, or remove the "Made with QORM" note — join **Indie**
(a solo dev / freelancer, US$1/mo) or **Studio** (a company, US$7/mo). This unlocks:

- **Custom app icon** — replace the QORM logo (`<app>/icon.png`).
- **Remove the "Made with QORM" note** from packaging metadata (`qorm package
  --no-branding`, or qorm.json `"branding": false`).
- **Commercial resale** — ship and sell the product.

It's honour-system: when you `qorm package` with a custom icon or `--no-branding`,
the CLI just asks you to confirm you're a subscriber — no verification, no keys.

## Supporter — back the project

The **Supporter** tier (US$3/mo) is for people who want to back QORM without
needing commercial white-labeling. It gets:

- **Priority feature requests** — open an issue (label `supporter`) or post on
  Patreon; reasonable requests from Supporters are prioritized.

(Supporter does **not** grant commercial white-labeling — that's Indie / Studio.)

## Supporters

Every patron — Supporter, Indie, or Studio — is a supporter of QORM, recognized on
the QORM Patreon page. Thank you.

---

*Good-faith branding condition only. Nothing here limits your rights to the
MIT-licensed source.*
