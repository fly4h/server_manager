import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Button, Form, Input, InputNumber, Modal, Popconfirm, Select, Space, Switch, Table, Tag, message } from 'antd'
import { CodeOutlined, PlusOutlined, ReloadOutlined, ThunderboltOutlined } from '@ant-design/icons'
import { api, ServerRow } from '../services/api'

type FormVals = {
  name: string
  host: string
  port: number
  username: string
  auth_type: 'password' | 'private_key'
  credential?: string
  description?: string
  tags?: string
  enabled?: boolean
}

const statusColor: Record<string, string> = {
  online: 'green', offline: 'red', unknown: 'default',
}

export default function Servers() {
  const [rows, setRows] = useState<ServerRow[]>([])
  const [loading, setLoading] = useState(false)
  const [modalOpen, setModalOpen] = useState(false)
  const [editing, setEditing] = useState<ServerRow | null>(null)
  const [testing, setTesting] = useState<number | null>(null)
  const [form] = Form.useForm()
  const nav = useNavigate()

  const load = async () => {
    setLoading(true)
    try {
      setRows((await api.get<ServerRow[]>('/api/servers')) ?? [])
    } catch { /* ignore */ } finally { setLoading(false) }
  }
  useEffect(() => { load() }, [])

  const openCreate = () => {
    setEditing(null)
    form.resetFields()
    form.setFieldsValue({ port: 22, username: 'root', auth_type: 'password', enabled: true })
    setModalOpen(true)
  }

  const openEdit = (row: ServerRow) => {
    setEditing(row)
    form.setFieldsValue({
      name: row.name, host: row.host, port: row.port, username: row.username,
      auth_type: row.auth_type, credential: undefined,
      description: row.description, tags: row.tags, enabled: row.enabled,
    })
    setModalOpen(true)
  }

  const submit = async () => {
    const vals = await form.validateFields()
    const body = { ...vals, credential: vals.credential || undefined }
    try {
      if (editing) await api.put(`/api/servers/${editing.id}`, body)
      else await api.post('/api/servers', body)
      message.success(editing ? '已更新' : '已添加')
      setModalOpen(false)
      load()
    } catch (e: any) {
      message.error(e.message)
    }
  }

  const test = async (row: ServerRow) => {
    setTesting(row.id)
    try {
      const res = await api.post<{ status: string; error?: string }>(`/api/servers/${row.id}/test`, {})
      if (res.status === 'online') message.success(`${row.name}: 连接成功`)
      else message.error(`${row.name}: ${res.error || '连接失败'}`)
      load()
    } catch (e: any) {
      message.error(e.message)
    } finally {
      setTesting(null)
    }
  }

  const remove = async (row: ServerRow) => {
    try {
      await api.del(`/api/servers/${row.id}`)
      message.success('已删除')
      load()
    } catch (e: any) { message.error(e.message) }
  }

  return (
    <div>
      <Space style={{ marginBottom: 16 }}>
        <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>添加服务器</Button>
        <Button icon={<ReloadOutlined />} onClick={load} loading={loading}>刷新</Button>
      </Space>
      <Table<ServerRow>
        rowKey="id"
        loading={loading}
        dataSource={rows}
        onRow={(row) => ({ onClick: () => nav(`/servers/${row.id}`), style: { cursor: 'pointer' } })}
        columns={[
          { title: '名称', dataIndex: 'name', render: (v, r) => <a onClick={() => nav(`/servers/${r.id}`)}>{v}</a> },
          { title: '主机', dataIndex: 'host', render: (v, r) => `${v}:${r.port}` },
          { title: '用户', dataIndex: 'username' },
          {
            title: '认证', dataIndex: 'auth_type',
            render: (v) => <Tag>{v === 'private_key' ? '私钥' : '密码'}</Tag>,
          },
          {
            title: '状态', dataIndex: 'last_status',
            render: (v) => <Tag color={statusColor[v] ?? 'default'}>{v}</Tag>,
          },
          { title: '描述', dataIndex: 'description', ellipsis: true },
          {
            title: '操作', key: 'actions', width: 320,
            render: (_, row) => (
              <Space onClick={(e) => e.stopPropagation()}>
                <Button size="small" icon={<CodeOutlined />} onClick={() => nav(`/servers/${row.id}?tab=terminal`)}>
                  终端
                </Button>
                <Button size="small" icon={<ThunderboltOutlined />} loading={testing === row.id} onClick={() => test(row)}>
                  测试
                </Button>
                <Button size="small" onClick={() => openEdit(row)}>编辑</Button>
                <Popconfirm title={`删除 ${row.name}？`} onConfirm={() => remove(row)}>
                  <Button size="small" danger>删除</Button>
                </Popconfirm>
              </Space>
            ),
          },
        ]}
      />

      <Modal
        title={editing ? `编辑服务器：${editing.name}` : '添加服务器'}
        open={modalOpen}
        onOk={submit}
        onCancel={() => setModalOpen(false)}
        destroyOnClose
        width={560}
      >
        <Form form={form} labelCol={{ span: 5 }} wrapperCol={{ span: 17 }}>
          <Form.Item name="name" label="名称" rules={[{ required: true }]}>
            <Input placeholder="server-01" />
          </Form.Item>
          <Form.Item name="host" label="主机" rules={[{ required: true }]}>
            <Input placeholder="192.168.1.100" />
          </Form.Item>
          <Form.Item name="port" label="端口" initialValue={22}>
            <InputNumber min={1} max={65535} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="username" label="用户名" initialValue="root">
            <Input />
          </Form.Item>
          <Form.Item name="auth_type" label="认证方式" initialValue="password">
            <Select options={[
              { value: 'password', label: '密码' },
              { value: 'private_key', label: '私钥' },
            ]} />
          </Form.Item>
          <Form.Item
            name="credential" label={form.getFieldValue('auth_type') === 'private_key' ? '私钥' : '密码'}
            extra={editing ? '留空表示不修改已保存的凭据' : undefined}
          >
            <Input.TextArea
              rows={form.getFieldValue('auth_type') === 'private_key' ? 4 : 1}
              placeholder={form.getFieldValue('auth_type') === 'private_key' ? '-----BEGIN OPENSSH PRIVATE KEY-----' : 'SSH 密码'}
              autoComplete="new-password"
            />
          </Form.Item>
          <Form.Item name="description" label="描述">
            <Input />
          </Form.Item>
          <Form.Item name="tags" label="标签">
            <Input placeholder='["prod","home"]' />
          </Form.Item>
          <Form.Item name="enabled" label="启用" valuePropName="checked" initialValue={true}>
            <Switch />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}
