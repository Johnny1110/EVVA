import { defineStore } from 'pinia'

// Session = the operator's connection identity: the service session token,
// minted fresh on every `evva service start` (RP-15). On the service's own
// machine the FE fetches it from the loopback-only bootstrap endpoint, so
// nobody types anything; from another device (--allow-remote) the operator
// pastes the minted token once (`evva service status` shows the token file).
const TOKEN_KEY = 'evva-swarm-token'

export const useSessionStore = defineStore('session', {
  state: () => ({ token: localStorage.getItem(TOKEN_KEY) || '' }),
  getters: {
    authed: (s): boolean => !!s.token,
  },
  actions: {
    // bootstrap asks the service for the session token. The endpoint answers
    // only loopback callers of a loopback-bound service (404 otherwise), so a
    // remote visitor silently falls through to the paste gate.
    async bootstrap(): Promise<void> {
      if (this.token) return
      try {
        const resp = await fetch('/api/auth/bootstrap')
        if (!resp.ok) return
        const { token } = (await resp.json()) as { token?: string }
        if (token) this.connect(token)
      } catch {
        // service unreachable — the paste gate stays up
      }
    },
    connect(t: string) {
      this.token = t.trim()
      localStorage.setItem(TOKEN_KEY, this.token)
    },
    disconnect() {
      this.token = ''
      localStorage.removeItem(TOKEN_KEY)
    },
  },
})
