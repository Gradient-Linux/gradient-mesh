# Gradient Mesh Repo Override

This repository owns the `gradient-mesh` daemon only.

- Branch: `feature/mesh`
- Socket path: `/run/gradient/mesh.sock`
- Config path: `~/gradient/config/mesh.json`
- Network-facing code must stay behind interfaces so tests can mock it.
- Do not touch sibling repositories or shared vault files from this repo.

