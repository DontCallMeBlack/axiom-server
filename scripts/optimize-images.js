const fs = require('fs');
const path = require('path');
const sharp = require('sharp');

if (process.argv.length < 4) {
  console.error('Usage: node optimize-images.js <input_dir> <output_dir>');
  process.exit(1);
}

const inDir = path.resolve(process.argv[2]);
const outDir = path.resolve(process.argv[3]);

if (!fs.existsSync(inDir)) {
  console.error('Input directory does not exist:', inDir);
  process.exit(1);
}

fs.mkdirSync(outDir, { recursive: true });

async function processFile(file) {
  const inPath = path.join(inDir, file);
  const ext = path.extname(file).toLowerCase();
  const base = path.basename(file, ext);
  const outPath = path.join(outDir, base + '.webp');

  try {
    const metadata = await sharp(inPath).metadata();
    const width = Math.min(metadata.width || 1200, 1200);

    await sharp(inPath)
      .resize({ width })
      .webp({ quality: 80 })
      .toFile(outPath);

    console.log('Optimized', file, '→', path.relative(process.cwd(), outPath));
  } catch (err) {
    console.error('Failed to process', file, err.message);
  }
}

(async () => {
  const files = fs.readdirSync(inDir).filter(f => !f.startsWith('.'));
  for (const f of files) {
    const full = path.join(inDir, f);
    const stat = fs.statSync(full);
    if (stat.isFile()) {
      await processFile(f);
    }
  }
  console.log('Done. Optimized', files.length, 'files.');
})();
