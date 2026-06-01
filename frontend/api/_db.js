import { MongoClient } from 'mongodb';

const uri = process.env.MONGO_URI || '';

let clientPromise;

if (!uri) {
  console.warn('MONGO_URI not set — API will run in-memory fallback mode');
} else {
  if (!globalThis._mongoClientPromise) {
    const client = new MongoClient(uri);
    globalThis._mongoClientPromise = client.connect();
  }
  clientPromise = globalThis._mongoClientPromise;
}

export default async function getDb() {
  if (!uri) return null;
  const client = await clientPromise;
  return client.db('axiom');
}
