import { useEffect, useState } from 'react'

// Another emailed code can be asked for this long after the last one.
const RESEND_SECONDS = 60

// The wait before another emailed code can be asked for (sign-up and password
// reset). Call `start` each time a code is sent; `seconds` then counts down to
// 0. It counts down to a moment rather than ticking off seconds, so it keeps
// time while the tab is in the background (where the person reads the email).
export function useResendWait(): { seconds: number; start: () => void } {
  const [until, setUntil] = useState(0)
  const [now, setNow] = useState(0)
  const seconds = Math.max(0, Math.ceil((until - now) / 1000))
  const waiting = seconds > 0

  useEffect(() => {
    if (!waiting) return
    const timer = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(timer)
  }, [waiting])

  return {
    seconds,
    start() {
      const at = Date.now()
      setNow(at)
      setUntil(at + RESEND_SECONDS * 1000)
    },
  }
}
