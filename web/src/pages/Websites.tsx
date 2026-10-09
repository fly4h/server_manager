import { useEffect, useState } from 'react'
import { Button, Card, Col, Form, Input, Modal, Popconfirm, Row, Select, Space, Switch, Tag, Typography, message } from 'antd'
import { EditOutlined, ExportOutlined, PlusOutlined, ReloadOutlined } from '@ant-design/icons'
import { api, WebsiteRow } from '../services/api'

const categories = [
  'general', 'storage', 'network', 'router', 'virtualization', 'home', 'dev', 'other',
]

const categoryColor: Record<string, string> = {
  general: 'blue', storage: 'cyan', network: 'geekblue', router: 'purple',
  virtualization: 'orange', home: 'green', dev: 'magenta', other: 'default',
}

export default function Websites() {
  const [rows, setRows] = useState<WebsiteRow[]>([])
  const [loading, setLoading] = useState(false)
  const [modalOpen, setModalOpen] = useState(false)
  const [editing, setEditing] = useState<WebsiteRow | null>(null)
  const [form] = Form.useForm()

  const load = async () => {
    setLoading(true)
    try {
      setRows((await api.get<WebsiteRow[]>('/api/websites')) ?? [])
    } catch { /* ignore */ } finally { setLoading(false) }
  }
  useEffect(() => { load() }, [])

  const openCreate = () => {
    setEditing(null)
    form.resetFields()
    form.setFieldsValue({ category: 'general', auth_type: 'none', enabled: true })
    setModalOpen(true)
  }

  const openEdit = (row: WebsiteRow) => {
    setEditing(row)
    form.setFieldsValue({
      name: row.name, url: row.url, icon: row.icon, category: row.category,
      auth_type: row.auth_type, credential: undefined, description: row.description, enabled: row.enabled,
    })
    setModalOpen(true)
  }

  const submit = async () => {
    const vals = await form.validateFields()
    const body = { ...vals, credential: vals.credential || undefined }
    try {
      if (editing) await api.put(`/api/websites/${editing.id}`, body)
      else await api.post('/api/websites', body)
      message.success(editing ? '已更新' : '已创建')
      setModalOpen(false)
      load()
    } catch (e: any) { message.error(e.message) }
  }

  const remove = async (row: WebsiteRow) => {
    try {
      await api.del(`/api/websites/${row.id}`)
      message.success('已删除')
      load()
    } catch (e: any) { message.error(e.message) }
  }

  return (
    <div>
      <Space style={{ marginBottom: 16 }}>
        <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>添加网站</Button>
        <Button icon={<ReloadOutlined />} onClick={load} loading={loading}>刷新</Button>
        <Typography.Text type="secondary">点击「打开」在新标签页访问；认证信息加密保存</Typography.Text>
      </Space>
      <Row gutter={[16, 16]}>
        {(rows ?? []).map((row) => (
          <Col key={row.id} xs={24} sm={12} md={8} lg={6}>
            <Card
              size="small"
              title={
                <Space>
                  <span>{row.icon || '🌐'}</span>
                  <span>{row.name}</span>
                  {!row.enabled && <Tag>已停用</Tag>}
                </Space>
              }
              extra={<Tag color={categoryColor[row.category] ?? 'default'}>{row.category}</Tag>}
              actions={[
                <Button
                  key="open" type="link" icon={<ExportOutlined />}
                  disabled={!row.enabled}
                  onClick={() => window.open(row.url, '_blank', 'noopener')}
                >
                  打开
                </Button>,
                <Button key="edit" type="link" icon={<EditOutlined />} onClick={() => openEdit(row)}>编辑</Button>,
                <Popconfirm key="del" title={`删除 ${row.name}？`} onConfirm={() => remove(row)}>
                  <Button type="link" danger>删除</Button>
                </Popconfirm>,
              ]}
            >
              <Typography.Paragraph ellipsis style={{ marginBottom: 4 }}>
                <a onClick={() => window.open(row.url, '_blank', 'noopener')}>{row.url}</a>
              </Typography.Paragraph>
              <Typography.Paragraph type="secondary" ellipsis style={{ marginBottom: 4, fontSize: 12 }}>
                {row.description || '—'}
              </Typography.Paragraph>
              <Space size={4}>
                {row.auth_type !== 'none' && <Tag color="gold">{row.auth_type}</Tag>}
                {row.credential_name && <Tag color="green">凭据已保存</Tag>}
              </Space>
            </Card>
          </Col>
        ))}
      </Row>

      <Modal
        title={editing ? `编辑网站：${editing.name}` : '添加网站'}
        open={modalOpen}
        onOk={submit}
        onCancel={() => setModalOpen(false)}
        destroyOnClose
        width={560}
      >
        <Form form={form} labelCol={{ span: 5 }} wrapperCol={{ span: 17 }}>
          <Form.Item name="name" label="名称" rules={[{ required: true }]}>
            <Input placeholder="OpenList" />
          </Form.Item>
          <Form.Item name="url" label="URL" rules={[{ required: true }, { type: 'url' }]}>
            <Input placeholder="https://openlist.example.com" />
          </Form.Item>
          <Form.Item name="icon" label="图标">
            <Input placeholder="emoji，如 📁" style={{ width: 80 }} />
          </Form.Item>
          <Form.Item name="category" label="分类" initialValue="general">
            <Select options={categories.map((c) => ({ value: c, label: c }))} />
          </Form.Item>
          <Form.Item name="auth_type" label="认证类型" initialValue="none">
            <Select options={[
              { value: 'none', label: 'none（无）' },
              { value: 'basic_auth', label: 'basic_auth' },
              { value: 'form', label: 'form（表单）' },
              { value: 'cookie', label: 'cookie' },
              { value: 'token', label: 'token' },
              { value: 'custom', label: 'custom' },
            ]} />
          </Form.Item>
          <Form.Item
            name="credential" label="认证信息"
            extra={editing ? '留空表示不修改已保存的凭据（V1 仅加密保存，不自动登录）' : 'V1 仅加密保存，不自动登录'}
          >
            <Input.TextArea rows={2} placeholder="密码 / Token / Cookie（AES-256-GCM 加密保存）" autoComplete="new-password" />
          </Form.Item>
          <Form.Item name="description" label="描述">
            <Input />
          </Form.Item>
          <Form.Item name="enabled" label="启用" valuePropName="checked" initialValue={true}>
            <Switch />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}
