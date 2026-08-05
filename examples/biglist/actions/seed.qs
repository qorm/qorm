// Seed the big list on scene enter: 500 generated rows so the virtualized
// list has something to render in every host (the demo's live path tops it
// up over the wire, but the canvas window starts from the manifest's empty
// initial state).
state.items = []
for i in range(500) {
  let tag = "prod"
  if (i % 3 == 0) { tag = "vip" }
  if (i % 3 == 1) { tag = "new" }
  state.items = concat(state.items, {"name": "Contact " + str(i + 1), "tag": tag})
}
