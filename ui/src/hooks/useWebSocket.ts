import { useEffect, useRef } from 'react'
import { useQueryClient } from '@tanstack/react-query'

// Topics broadcast by the server map to multiple queryKey prefixes.
const TOPIC_KEYS: Record<string, string[][]> = {
  hosts:        [['hosts'], ['host']],
  policies:     [['policies'], ['policy']],
  objectgroups: [['objectgroups']],
  tokens:       [['tokens']],
  users:        [['users']],
  'audit-log':  [['audit-log']],
  'default-policy': [['default-policy']],
}

export function useWebSocket() {
  const qc = useQueryClient()
  const retryRef = useRef(0)
  const destroyedRef = useRef(false)

  useEffect(() => {
    destroyedRef.current = false

    function connect() {
      const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
      const ws = new WebSocket(`${proto}//${window.location.host}/ws`)

      ws.onopen = () => { retryRef.current = 0 }

      ws.onmessage = (e) => {
        try {
          const { topic } = JSON.parse(e.data) as { topic: string }
          const keys = TOPIC_KEYS[topic]
          if (keys) keys.forEach(k => qc.invalidateQueries({ queryKey: k }))
        } catch { /* ignore malformed */ }
      }

      ws.onclose = () => {
        if (destroyedRef.current) return
        const delay = Math.min(500 * 2 ** retryRef.current, 30_000)
        retryRef.current++
        setTimeout(connect, delay)
      }

      ws.onerror = () => ws.close()

      return ws
    }

    const ws = connect()
    return () => {
      destroyedRef.current = true
      ws.close()
    }
  }, [qc])
}
