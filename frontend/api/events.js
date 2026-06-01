import getDb from './_db.js';

export default async function handler(req, res) {
  const since = req.query.since ? new Date(req.query.since) : new Date(0);
  const db = await getDb();
  if (!db) return res.json([]);
  const events = db.collection('events');
  const rows = await events.find({ ts: { $gt: since } }).sort({ ts: 1 }).toArray();
  res.json(rows);
}
