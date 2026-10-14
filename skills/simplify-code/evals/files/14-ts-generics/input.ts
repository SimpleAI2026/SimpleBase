// shared util, used by reports + dashboard
export function groupBy<T, K extends string | number>(items: T[], key: (t: T) => K): Record<string, T[]> {
  var out: any = {};
  for (var i = 0; i < items.length; i++) {
    var k = key(items[i]);
    if (!out[k]) out[k] = [];
    out[k].push(items[i]);
  }
  return out;
}

export function sortedKeys(rec: Record<string, unknown>): string[] {
  var ks: string[] = [];
  for (var k in rec) ks.push(k);
  return ks.sort();
}
