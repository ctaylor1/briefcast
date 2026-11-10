// Briefcast record IDs are UUIDs (db.Base sets uuid.NewString()).
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

export function isRecordId(value: unknown): value is string {
  return typeof value === "string" && UUID.test(value);
}

// Splits comma-separated ID query values, trims them and keeps only valid, unique IDs.
export function parseRecordIds(...raw: unknown[]): string[] {
  const ids: string[] = [];
  const add = (value: unknown) => {
    if (typeof value !== "string") {
      return;
    }
    for (const part of value.split(",")) {
      const id = part.trim();
      if (isRecordId(id) && !ids.includes(id)) {
        ids.push(id);
      }
    }
  };
  for (const value of raw) {
    if (Array.isArray(value)) {
      value.forEach(add);
    } else {
      add(value);
    }
  }
  return ids;
}
