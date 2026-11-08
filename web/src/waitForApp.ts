// How the page waits for Mediarium to come back after it restarts.
export interface WaitOptions {
  expectVersion?: string
  fetchVersion: () => Promise<string | null> // the running version, null while it does not answer
  sleep: (ms: number) => Promise<void>
  now: () => number
  minMs?: number // do not believe an answer before this: the old process may still be up
  maxMs?: number
  everyMs?: number
}

// Resolves 'back' once the app answers (with the expected version, if one is
// given), or 'timeout'.
export async function waitForApp(o: WaitOptions): Promise<'back' | 'timeout'> {
  const start = o.now()
  const minMs = o.minMs ?? 4000
  const maxMs = o.maxMs ?? 150000
  const every = o.everyMs ?? 2000
  for (;;) {
    await o.sleep(every)
    const elapsed = o.now() - start
    if (elapsed > maxMs) return 'timeout'
    const v = await o.fetchVersion()
    if (v !== null && elapsed >= minMs && (!o.expectVersion || v === o.expectVersion)) return 'back'
  }
}
