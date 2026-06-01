import getDb from './_db.js';

export default async function handler(req, res) {
  if (req.method !== 'POST') return res.status(405).end();
  const { username, password, char_name, game_class } = req.body || {};
  if (!username || !char_name) return res.status(400).json({ error: 'username and char_name required' });

  const db = await getDb();
  if (!db) {
    // fallback: return success but no persistence
    return res.json({ status: 'success', note: 'running without DB' });
  }

  const users = db.collection('users');
  const roster = db.collection('roster');
  const existing = await users.findOne({ username });
  if (existing) return res.status(409).json({ error: 'user already exists' });

  const u = { username, password, char_name, game_class, role: 'member' };
  await users.insertOne(u);
  await roster.updateOne({ char_name }, { $set: { char_name, game_class } }, { upsert: true });
  await db.collection('escrow').updateOne({ char_name }, { $setOnInsert: { char_name, amount: 0 } }, { upsert: true });

  res.json({ status: 'success' });
}
