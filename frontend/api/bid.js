import getDb from './_db.js';
import { pooledActivityFor } from './_helpers.js';

export default async function handler(req, res) {
  if (req.method !== 'POST') return res.status(405).end();
  const { auction_id, char_name, bid_amount } = req.body || {};
  const db = await getDb();
  if (!db) return res.status(200).json({ status: 'success', note: 'no-db' });

  const auctions = db.collection('auctions');
  const escrow = db.collection('escrow');
  const events = db.collection('events');

  const auction = await auctions.findOne({ id: auction_id });
  if (!auction || !auction.isActive) return res.status(400).json({ error: 'Auction not found or closed.' });

  const now = new Date();
  if (now > new Date(auction.endTime)) return res.status(403).json({ error: 'Auction has ended.' });

  const roster = db.collection('roster');
  const r = await roster.findOne({ char_name });
  if (!r) return res.status(403).json({ error: 'Character not found in the database roster.' });

  if (auction.class_restriction && auction.class_restriction !== 'all' && r.game_class !== auction.class_restriction) {
    return res.status(403).json({ error: 'Class mismatch' });
  }

  if (pooledActivityFor(char_name) < 300) return res.status(403).json({ error: 'Bidding Locked: Under 300 DKP pooled activity in the last 7 days.' });

  const playerTotalBank = 450;
  const curEsc = (await escrow.findOne({ char_name })) || { amount: 0 };
  const available = playerTotalBank - (curEsc.amount || 0);

  let bidDiff = bid_amount;
  if (auction.highestBidder === char_name) bidDiff = bid_amount - (auction.highestBid || 0);
  if (bidDiff > available) return res.status(403).json({ error: 'Insufficient DKP' });
  if (bid_amount <= (auction.highestBid || 0)) return res.status(400).json({ error: 'Bid must be higher than current bid.' });

  // Anti-snipe: if remaining <= 5 minutes extend by 5 minutes
  const timeRemaining = new Date(auction.endTime) - now;
  let extensionTriggered = false;
  if (timeRemaining <= 5*60*1000) {
    const newEnd = new Date(new Date(auction.endTime).getTime() + 5*60*1000);
    await auctions.updateOne({ id: auction_id }, { $set: { endTime: newEnd } });
    extensionTriggered = true;
  }

  if (auction.highestBidder && auction.highestBidder !== char_name && auction.highestBidder !== 'None') {
    await escrow.updateOne({ char_name: auction.highestBidder }, { $inc: { amount: -(auction.highestBid || 0) } });
  }

  await auctions.updateOne({ id: auction_id }, { $set: { highestBid: bid_amount, highestBidder: char_name } });
  await escrow.updateOne({ char_name }, { $inc: { amount: bidDiff } }, { upsert: true });
  await events.insertOne({ type: 'new_bid', auction_id, char_name, amount: bid_amount, extended: extensionTriggered, ts: new Date() });

  res.json({ status: 'success' });
}
