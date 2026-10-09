import { useEffect, useState } from 'react'
import { Card, Col, Descriptions, Form, Input, Row, Tabs, Button, message } from 'antd'
import { api, CredentialRow } from '../services/api'
import { useAuth } from '../stores/auth'

type SysInfo = {
  version: string
  go_version: string
  env: string
  db_size: number
  docker_ok: boolean
  docker_host: string
}

function Account() {
  const { user, refresh } = useAuth()
  const [form] = Form.useForm()

  const submit = async () => {
    const vals = await form.validateFields()
    if (vals.new_password !== vals.confirm) {
      message.error('两次输入的新密码不一致')
      return
    }
    try {
      await api.put('/api/auth/password', { old_password: vals.old_password, new_password: vals.new_password })
      message.success('密码已修改')
      form.resetFields()
      refresh()
    } catch (e: any) {
      message.error(e.message)
    }
  }

  return (
    <Card title="账号" style={{ maxWidth: 520 }}>
      <Descriptions column={1} size="small" style={{ marginBottom: 24 }}>
        <Descriptions.Item label="用户名">{user?.username}</Descriptions.Item>
        <Descriptions.Item label="角色">{user?.role}</Descriptions.Item>
      </Descriptions>
      <Form form={form} layout="vertical" style={{ maxWidth: 360 }}>
        <Form.Item name="old_password" label="当前密码" rules={[{ required: true }]}>
          <Input.Password autoComplete="current-password" />
        </Form.Item>
        <Form.Item name="new_password" label="新密码" rules={[{ required: true }, { min: 8, message: '至少 8 个字符' }]}>
          <Input.Password autoComplete="new-password" />
        </Form.Item>
        <Form.Item name="confirm" label="确认新密码" rules={[{ required: true }]}>
          <Input.Password autoComplete="new-password" />
        </Form.Item>
        <Button type="primary" onClick={submit}>修改密码</Button>
      </Form>
    </Card>
  )
}

function System() {
  const [info, setInfo] = useState<SysInfo | null>(null)
  useEffect(() => {
    api.get<SysInfo>('/api/system/info').then(setInfo).catch(() => {})
  }, [])

  return (
    <Card title="系统" style={{ maxWidth: 520 }}>
      <Descriptions column={1} size="small" bordered>
        <Descriptions.Item label="版本">{info?.version ?? '-'}</Descriptions.Item>
        <Descriptions.Item label="Go">{info?.go_version ?? '-'}</Descriptions.Item>
        <Descriptions.Item label="环境">{info?.env ?? '-'}</Descriptions.Item>
        <Descriptions.Item label="数据库大小">
          {info ? (info.db_size / 1024).toFixed(1) + ' KB' : '-'}
        </Descriptions.Item>
        <Descriptions.Item label="Docker">
          {info?.docker_ok ? <span style={{ color: '#3f8600' }}>已连接（{info.docker_host}）</span> : '未连接'}
        </Descriptions.Item>
      </Descriptions>
    </Card>
  )
}

function Credentials() {
  const [rows, setRows] = useState<CredentialRow[]>([])
  useEffect(() => {
    api.get<CredentialRow[]>('/api/credentials').then((r) => setRows(r ?? [])).catch(() => {})
  }, [])
  return (
    <Card title="凭据（仅元数据，密文不出库）" style={{ maxWidth: 720 }}>
      {rows.length === 0 ? (
        <span>暂无独立凭据。服务器/网站保存密码时会自动创建加密凭据。</span>
      ) : (
        <ul style={{ paddingLeft: 20 }}>
          {rows.map((c) => (
            <li key={c.id}>
              <b>{c.name}</b> · <code>{c.type}</code> · {c.created_at}
            </li>
          ))}
        </ul>
      )}
    </Card>
  )
}

export default function Settings() {
  return (
    <div>
      <Row gutter={16}>
        <Col span={24}>
          <Tabs
            items={[
              { key: 'account', label: '账号', children: <Account /> },
              { key: 'system', label: '系统', children: <System /> },
              { key: 'credentials', label: '凭据', children: <Credentials /> },
            ]}
          />
        </Col>
      </Row>
    </div>
  )
}
