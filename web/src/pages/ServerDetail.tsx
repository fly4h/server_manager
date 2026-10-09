import { useEffect, useMemo, useState } from 'react'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import {
  Alert, Button, Card, Descriptions, Form, Input, Modal, Space, Spin, Tabs, Tag, Typography, message,
} from 'antd'
import { ArrowLeftOutlined, PlayCircleOutlined, ReloadOutlined } from '@ant-design/icons'
import { api, CommandRow, ServerRow } from '../services/api'
import TerminalPane from '../components/Terminal'

const dangerColor: Record<string, string> = {
  low: 'green', medium: 'orange', high: 'red', critical: 'magenta',
}

/** Extract {{param}} names from a command template. */
function extractParams(cmd: string): string[] {
  const names = new Set<string>()
  for (const m of cmd.matchAll(/\{\{\s*([a-zA-Z_][a-zA-Z0-9_]*)\s*\}\}/g)) names.add(m[1])
  return [...names]
}

export default function ServerDetail() {
  const { id } = useParams()
  const [sp] = useSearchParams()
  const nav = useNavigate()
  const [server, setServer] = useState<ServerRow | null>(null)
  const [commands, setCommands] = useState<CommandRow[]>([])
  const [loading, setLoading] = useState(true)
  const [execCmd, setExecCmd] = useState<CommandRow | null>(null)
  const [params, setParams] = useState<Record<string, string>>({})
  const [result, setResult] = useState<{ output: string; error: string; command: string; danger_level: string } | null>(null)
  const [executing, setExecuting] = useState(false)
  const [form] = Form.useForm()

  const load = async () => {
    if (!id) return
    setLoading(true)
    try {
      const [s, c] = await Promise.all([
        api.get<ServerRow>(`/api/servers/${id}`),
        api.get<CommandRow[]>('/api/commands'),
      ])
      setServer(s)
      setCommands(c ?? [])
    } catch (e: any) {
      message.error(e.message)
    } finally { setLoading(false) }
  }
  useEffect(() => { load() }, [id])

  const paramNames = useMemo(() => (execCmd ? extractParams(execCmd.command) : []), [execCmd])

  const openExec = (cmd: CommandRow) => {
    setExecCmd(cmd)
    setResult(null)
    const empty: Record<string, string> = {}
    for (const p of extractParams(cmd.command)) empty[p] = ''
    setParams(empty)
  }

  const execute = async (confirm: boolean) => {
    if (!execCmd || !id) return
    setExecuting(true)
    try {
      const res = await api.post<{ output: string; error: string; command: string; danger_level: string }>(
        `/api/servers/${id}/commands/${execCmd.id}/execute`,
        { params, confirm },
      )
      setResult(res)
    } catch (e: any) {
      if (e.code === 'CONFIRMATION_REQUIRED') {
        // show second confirmation with the rendered command
        Modal.confirm({
          title: `高危命令确认（${e.data?.danger_level}）`,
          content: (
            <div>
              <Alert type="error" message="该命令风险较高，请确认后执行：" style={{ marginBottom: 12 }} />
              <Typography.Paragraph code copyable style={{ whiteSpace: 'pre-wrap' }}>
                {e.data?.command}
              </Typography.Paragraph>
            </div>
          ),
          okText: '确认执行',
          okButtonProps: { danger: true },
          cancelText: '取消',
          onOk: () => execute(true),
        })
      } else {
        message.error(e.message)
      }
    } finally {
      setExecuting(false)
    }
  }

  if (loading) return <div style={{ textAlign: 'center', padding: 64 }}><Spin /></div>
  if (!server) return <Alert type="error" message="服务器不存在" />

  const tab = sp.get('tab') ?? 'overview'

  return (
    <div>
      <Space style={{ marginBottom: 16 }}>
        <Button icon={<ArrowLeftOutlined />} onClick={() => nav('/servers')}>返回</Button>
        <Typography.Title level={4} style={{ margin: 0 }}>{server.name}</Typography.Title>
        <Tag color={server.last_status === 'online' ? 'green' : server.last_status === 'offline' ? 'red' : 'default'}>
          {server.last_status}
        </Tag>
      </Space>

      <Tabs
        activeKey={tab}
        onChange={(k) => nav(`/servers/${server.id}?tab=${k}`, { replace: true })}
        items={[
          {
            key: 'overview',
            label: '概览',
            children: (
              <Card>
                <Descriptions column={2} bordered size="small">
                  <Descriptions.Item label="主机">{server.host}:{server.port}</Descriptions.Item>
                  <Descriptions.Item label="用户">{server.username}</Descriptions.Item>
                  <Descriptions.Item label="认证方式">
                    {server.auth_type === 'private_key' ? '私钥' : '密码'}
                  </Descriptions.Item>
                  <Descriptions.Item label="凭据">{server.credential_name ?? '—'}</Descriptions.Item>
                  <Descriptions.Item label="描述">{server.description || '—'}</Descriptions.Item>
                  <Descriptions.Item label="标签">{server.tags || '—'}</Descriptions.Item>
                  <Descriptions.Item label="最近检查">{server.last_check_at ?? '—'}</Descriptions.Item>
                  <Descriptions.Item label="创建时间">{server.created_at}</Descriptions.Item>
                </Descriptions>
              </Card>
            ),
          },
          {
            key: 'terminal',
            label: '终端',
            children: (
              <div style={{ height: 'calc(100vh - 220px)', minHeight: 420 }}>
                <TerminalPane serverId={server.id} title={`${server.username}@${server.host}`} />
              </div>
            ),
          },
          {
            key: 'commands',
            label: '命令',
            children: (
              <Card>
                <Space style={{ marginBottom: 12 }}>
                  <Button icon={<ReloadOutlined />} onClick={load}>刷新</Button>
                  <Typography.Text type="secondary">点击执行命令模板，高危命令需二次确认</Typography.Text>
                </Space>
                {commands.filter((c) => c.enabled).length === 0 ? (
                  <Typography.Text type="secondary">还没有命令模板，请先到「命令」页面创建。</Typography.Text>
                ) : (
                  commands.filter((c) => c.enabled).map((cmd) => (
                    <Card
                      key={cmd.id}
                      size="small"
                      style={{ marginBottom: 8 }}
                      actions={[
                        <Button
                          key="run" type="link" icon={<PlayCircleOutlined />}
                          onClick={() => openExec(cmd)}
                        >
                          执行
                        </Button>,
                      ]}
                    >
                      <Card.Meta
                        title={
                          <Space>
                            {cmd.name}
                            <Tag color={dangerColor[cmd.danger_level]}>{cmd.danger_level}</Tag>
                          </Space>
                        }
                        description={
                          <Typography.Text code style={{ whiteSpace: 'pre-wrap' }}>{cmd.command}</Typography.Text>
                        }
                      />
                    </Card>
                  ))
                )}
              </Card>
            ),
          },
        ]}
      />

      <Modal
        title={`执行命令：${execCmd?.name ?? ''}`}
        open={!!execCmd}
        onCancel={() => setExecCmd(null)}
        footer={null}
        destroyOnClose
      >
        {execCmd && (
          <Form form={form} layout="vertical">
            <Typography.Paragraph code copyable style={{ whiteSpace: 'pre-wrap' }}>
              {execCmd.command}
            </Typography.Paragraph>
            {paramNames.map((p) => (
              <Form.Item key={p} label={p} required>
                <Input
                  value={params[p]}
                  onChange={(e) => setParams((prev) => ({ ...prev, [p]: e.target.value }))}
                  placeholder={`参数 ${p}`}
                />
              </Form.Item>
            ))}
            <Space>
              <Button type="primary" icon={<PlayCircleOutlined />} loading={executing} onClick={() => execute(false)}>
                执行
              </Button>
              <Button onClick={() => setExecCmd(null)}>取消</Button>
            </Space>
          </Form>
        )}
        {result && (
          <div style={{ marginTop: 16 }}>
            <Space style={{ marginBottom: 8 }}>
              <Tag color={dangerColor[result.danger_level]}>{result.danger_level}</Tag>
              <Typography.Text code copyable>{result.command}</Typography.Text>
            </Space>
            {result.error && <Alert type="error" message={result.error} style={{ marginBottom: 8 }} />}
            <pre
              style={{
                background: '#1e1e1e', color: '#d4d4d4', padding: 12, borderRadius: 6,
                maxHeight: 320, overflow: 'auto', fontSize: 12,
              }}
            >
              {result.output || '(无输出)'}
            </pre>
          </div>
        )}
      </Modal>
    </div>
  )
}
