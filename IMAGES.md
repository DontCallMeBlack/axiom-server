Image workflow

Place raw loot images in the repository root `loot-images/` and run the optimizer to generate web-optimized images in `frontend/public/loot-images/`.

Prerequisites:
- Node.js (16+ recommended)
- npm

Install dependencies:

```bash
npm install
```

Optimize images (reads `./loot-images` and writes `./frontend/public/loot-images`):

```bash
npm run optimize-images
```

Notes:
- Output images are converted to WebP and resized to a maximum width of 1200px at quality 80.
- Commit the generated images in `frontend/public/loot-images` to serve them via Vercel's static CDN.
- For large collections, optimize locally before committing to avoid large git history.
