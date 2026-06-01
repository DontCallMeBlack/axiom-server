import fs from 'fs';
import path from 'path';

export default async function handler(req, res) {
  const dir = path.join(process.cwd(), '..', 'loot-images');
  const publicDir = path.join(process.cwd(), 'public', 'loot-images');
  const loot = [];
  try {
    const files = fs.readdirSync(publicDir);
    for (const f of files) {
      if (fs.statSync(path.join(publicDir, f)).isFile()) {
        const ext = path.extname(f);
        const clean = f.replace(ext, '').replace(/_/g, ' ');
        loot.push({ name: clean, image_url: '/loot-images/' + f });
      }
    }
  } catch (e) {
    // fallback: try root loot-images
    try {
      const files = fs.readdirSync(path.join(process.cwd(), '..', 'loot-images'));
      for (const f of files) {
        loot.push({ name: f, image_url: '/loot-images/' + f });
      }
    } catch (e2) {}
  }
  res.json(loot);
}
