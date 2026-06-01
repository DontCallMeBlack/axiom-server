import getDb from './_db.js';

export default async function handler(req, res) {
  const db = await getDb();
  if (!db) return res.json([]);
  const roster = db.collection('roster');
  const escrow = db.collection('escrow');
  const users = db.collection('users');

  const entries = [];
  const rosterRows = await roster.find({}).toArray();
  for (const r of rosterRows) {
    const totalBank = 450;
    const inEscrow = (await escrow.findOne({ char_name: r.char_name })) || { amount: 0 };
    const user = await users.findOne({ char_name: r.char_name }) || {};
    entries.push({ role: user.role || 'Clansman', char_name: r.char_name, game_class: r.game_class, pooled_activity: 100, total_bank: totalBank, in_escrow: inEscrow.amount || 0, available_dkp: totalBank - (inEscrow.amount || 0) });
  }
  res.json(entries);
}
