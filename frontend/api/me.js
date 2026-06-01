import getDb from './_db.js';

export default async function handler(req, res) {
  const q = req.query.user || 'chief';
  const db = await getDb();
  if (!db) return res.status(200).json({ username: 'chief', char_name: 'Chief', game_class: 'officer', role: 'chief' });
  const users = db.collection('users');
  const u = await users.findOne({ username: q });
  if (!u) return res.status(404).json({ error: 'no user found' });
  res.json({ username: u.username, char_name: u.char_name, game_class: u.game_class, role: u.role });
}
