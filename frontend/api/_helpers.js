// Shared helpers for serverless API
export function pooledActivityFor(name) {
  let sum = 0;
  for (let i = 0; i < name.length; i++) sum += name.charCodeAt(i);
  return 100 + (sum % 400);
}

export function nowISO() {
  return new Date().toISOString();
}
