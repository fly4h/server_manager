import { useEffect, useRef, useState } from 'react'
import { Button, Space, Tag, Typography } from 'antd'
import { ApiOutlined, ReloadOutlined } from '@ant-design/icons'
import { Terminal as XTerm } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'

type Props = { serverId: number; title?: string }

type Status = 'connecting' | 'connected' | 'closed'

/**
 * SSH terminal over WebSocket.
 * Protocol: {"type":"input"|"resize"} out, {"type":"output"|"error"|"closed"} in.
 */
export default function TerminalPane({ serverId, title }: Props) {
  const holderRef = useRef<HTMLDivElement>(null)
  const [status, setStatus] = useState<Status>('connecting')
  const [nonce, setNonce] = useState(0) // bump to reconnect
  const wsRef = useRef<WebSocket | null>(null)

  useEffect(() => {
    const holder = holderRef.current
    if (!holder) return

    const term = new XTerm({
      cursorBlink: true,
      fontSize: 13,
      fontFamily: 'Consolas, "Courier New", monospace',
      theme: { background: '#1e1e1e', foreground: '#d4d4d4' },
      scrollback: 5000,
    })
    const fit = new FitAddon()
    term.loadAddon(fit)
    term.open(holder)
    try { fit.fit() } catch { /* holder may be hidden initially */ }

    const proto = location.protocol === 'https:' ? 'wss' : 'ws'
    const ws = new WebSocket(`${proto}://${location.host}/api/servers/${serverId}/terminal`)
    wsRef.current = ws
    setStatus('connecting')

    let closedByServer = false
    ws.onopen = () => {
      setStatus('connected')
      term.focus()
    }
    ws.onmessage = (ev) => {
      try {
        const msg = JSON.parse(ev.data)
        if (msg.type === 'output') term.write(msg.data)
        else if (msg.type === 'error') term.write(`\r\n\x1b[31m${msg.message}\x1b[0m\r\n`)
        else if (msg.type === 'closed') {
          closedByServer = true
          setStatus('closed')
          term.write('\r\n\x1b[31m[连接已关闭]\x1b[0m\r\n')
        }
      } catch { /* ignore malformed */ }
    }
    ws.onclose = () => {
      if (!closedByServer) {
        setStatus('closed')
        term.write('\r\n\x1b[31m[连接断开]\x1b[0m\r\n')
      }
    }

    const send = (obj: unknown) => {
      if (ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify(obj))
    }
    term.onData((data) => send({ type: 'input', data }))

    // Ctrl+Shift+C / V for copy/paste; plain selection also copies in most browsers.
    term.attachCustomKeyEventHandler((e) => {
      if (e.ctrlKey && e.shiftKey && (e.key === 'C' || e.key === 'c')) {
        const sel = term.getSelection()
        if (sel) navigator.clipboard?.writeText(sel)
        return false
      }
      if (e.ctrlKey && e.shiftKey && (e.key === 'V' || e.key === 'v')) {
        navigator.clipboard?.readText().then((t) => send({ type: 'input', data: t }))
        return false
      }
      if (e.ctrlKey && e.shiftKey && (e.key === 'F' || e.key === 'f')) {
        // fullscreen toggle is handled below via keydown on window
      }
      return true
    })

    const sendResize = () => {
      const cols = term.cols
      const rows = term.rows
      send({ type: 'resize', cols, rows })
    }
    const ro = new ResizeObserver(() => {
      try { fit.fit() } catch { /* ignore */ }
      sendResize()
    })
    ro.observe(holder)

    const onFullscreen = (e: KeyboardEvent) => {
      if (e.ctrlKey && e.shiftKey && (e.key === 'F' || e.key === 'f')) {
        holder.requestFullscreen?.()
      }
    }
    window.addEventListener('keydown', onFullscreen)

    return () => {
      window.removeEventListener('keydown', onFullscreen)
      ro.disconnect()
      ws.onclose = null
      ws.close()
      wsRef.current = null
      term.dispose()
    }
  }, [serverId, nonce])

  const statusTag = (
    <Tag color={status === 'connected' ? 'green' : status === 'connecting' ? 'blue' : 'red'}>
      {status === 'connected' ? '已连接' : status === 'connecting' ? '连接中…' : '已断开'}
    </Tag>
  )

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      <Space style={{ marginBottom: 8 }}>
        <ApiOutlined />
        <Typography.Text strong>{title ?? `SSH Terminal #${serverId}`}</Typography.Text>
        {statusTag}
        <Button
          size="small"
          icon={<ReloadOutlined />}
          disabled={status !== 'closed'}
          onClick={() => setNonce((n) => n + 1)}
        >
          重新连接
        </Button>
        <Typography.Text type="secondary" style={{ fontSize: 12 }}>
          Ctrl+Shift+C 复制 · Ctrl+Shift+V 粘贴 · Ctrl+Shift+F 全屏
        </Typography.Text>
      </Space>
      <div
        ref={holderRef}
        style={{
          flex: 1,
          minHeight: 380,
          background: '#1e1e1e',
          borderRadius: 6,
          padding: 6,
          overflow: 'hidden',
        }}
      />
    </div>
  )
}
