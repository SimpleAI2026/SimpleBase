export interface Step<T> { name: string; run: () => Promise<T>; }

// runs migration steps; returns names of the steps that completed
export async function runAll<T>(steps: Step<T>[], log: (s: string) => void): Promise<string[]> {
  const done: string[] = [];
  for (let i = 0; i < steps.length; i++) {
    const s = steps[i];
    log('start ' + s.name);
    try {
      await s.run();
    } catch (e) {
      log('fail ' + s.name);
      return done;
    }
    done.push(s.name);
  }
  return done;
}
