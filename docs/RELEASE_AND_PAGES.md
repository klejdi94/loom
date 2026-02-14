# Release and GitHub Pages

## Create the first version (v1.0.0)

1. Create and push a tag:
   ```bash
   git tag v1.0.0
   git push origin v1.0.0
   ```
2. The **Release** workflow runs: builds the repo, runs tests, builds CLI binaries for Linux, macOS (amd64/arm64), and Windows, then creates a GitHub Release with those artifacts and the tag.

## Enable GitHub Pages

1. In the repo: **Settings** → **Pages**.
2. Under **Build and deployment**, set **Source** to **GitHub Actions**.
3. The **Pages** workflow runs on every push to `main` (or `master`) and deploys the contents of the `docs/` folder.
4. After the first successful run, the site is available at **https://klejdi94.github.io/loom**.

To trigger Pages manually: **Actions** → **Pages** → **Run workflow**.
