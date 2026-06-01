import getDb from './_db.js';

export default async function handler(req, res) {
  if (req.method !== 'POST') return res.status(405).end();
  const { item_name, image_url, holder_name, class_restriction } = req.body || {};
  const db = await getDb();
  if (!db) return res.status(200).json({ status: 'success', note: 'no-db' });
  const auctions = db.collection('auctions');
  const id = `auc-${Date.now()}`;
  const doc = { id, item_name, image_url, holder_name, class_restriction, endTime: new Date(Date.now() + 24*60*60*1000), highestBid: 0, highestBidder: 'None', isActive: true };
  await auctions.insertOne(doc);
  await db.collection('events').insertOne({ type: 'auction_started', auction: doc, ts: new Date() });
  res.json({ status: 'success' });
}
