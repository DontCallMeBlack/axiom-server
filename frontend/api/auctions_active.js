import getDb from './_db.js';
import { nowISO } from './_helpers.js';

async function finalizeDueAuctions(db) {
  const auctions = db.collection('auctions');
  const ledger = db.collection('ledger');
  const escrow = db.collection('escrow');
  const events = db.collection('events');

  const now = new Date();
  const due = await auctions.find({ isActive: true, endTime: { $lte: now } }).toArray();
  for (const a of due) {
    await auctions.updateOne({ _id: a._id }, { $set: { isActive: false } });
    if (a.highestBidder && a.highestBidder !== 'None') {
      const winner = a.highestBidder;
      const amt = a.highestBid || 0;
      await escrow.updateOne({ char_name: winner }, { $inc: { amount: -amt } });
      await ledger.insertOne({ id: `led-${Date.now()}`, char_name: winner, amount: -amt, type: 'auction_spend', timestamp: new Date(), auction_id: a.id });
      await events.insertOne({ type: 'auction_finalized', auction_id: a.id, winner, amount: amt, ts: new Date() });
    }
  }
}

export default async function handler(req, res) {
  const db = await getDb();
  if (!db) return res.json([]);
  await finalizeDueAuctions(db);
  const auctions = db.collection('auctions');
  const now = new Date();
  const active = await auctions.find({ isActive: true }).toArray();
  res.json(active.map(a => ({ id: a.id, item_name: a.item_name, image_url: a.image_url, holder_name: a.holder_name, class_restriction: a.class_restriction, end_time: a.endTime, highest_bid: a.highestBid, highest_bidder: a.highestBidder, is_active: a.isActive })));
}
