import { useEffect, useState } from 'react'
import { Button, Form, Input, Modal, Popconfirm, Select, Space, Switch, Table, Tag, Typography, message } from 'antd'
import { PlusOutlined, ReloadOutlined } from '@ant-design/icons'
import { api, CommandRow } from '../services/api'

const dangerColor: Record<string, string> = {
  low: 'green', medium: 'orange', high: 'red', critical: 'magenta',
}

function extractParams(cmd: string): string[] {
  const names = new Set<string>()
  for (const m of cmd.matchAll(/\{\{\s*([a-zA-Z_][a-zA-Z0-9_]*)\s*\}\}/g)) names.add(m[1])
  return [...names]
}

export default function Commands() {
  const [rows, setRows] = useState<CommandRow[]>([])
  const [loading, setLoading] = useState(false)
  const [modalOpen, setModalOpen] = useState(false)
  const [editing, setEditing] = useState<CommandRow | null>(null)
  const [form] = Form.useForm()

  const load = async () => {
    setLoading(true)
    try {
      setRows((await api.get<CommandRow[]>('/api/commands')) ?? [])
    } catch { /* ignore */ } finally { setLoading(false) }
  }
  useEffect(() => { load() }, [])

  const openCreate = () => {
    setEditing(null)
    form.resetFields()
    form.setFieldsValue({ danger_level: 'low', enabled: true })
    setModalOpen(true)
  }

  const openEdit = (row: CommandRow) => {
    setEditing(row)
    form.setFieldsValue({
      name: row.name, description: row.description, command: row.command,
      danger_level: row.danger_level, enabled: row.enabled,
    })
    setModalOpen(true)
  }

  const submit = async () => {
    const vals = await form.validateFields()
    try {
      if (editing) await api.put(`/api/commands/${editing.id}`, vals)
      else await api.post('/api/commands', vals)
      message.success(editing ? '已更新' : '已创建')
      setModalOpen(false)
      load()
    } catch (e: any) { message.error(e.message) }
  }

  const remove = async (row: CommandRow) => {
    try {
      await api.del(`/api/commands/${row.id}`)
      message.success('已删除')
      load()
    } catch (e: any) { message.error(e.message) }
  }

  const commandValue = Form.useWatch('command', form)

  return (
    <div>
      <Space style={{ marginBottom: 16 }}>
        <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>新建命令</Button>
        <Button icon={<ReloadOutlined />} onClick={load} loading={loading}>刷新</Button>
        <Typography.Text type="secondary">模板支持 {'{{param}}'} 参数，执行时会校验输入</Typography.Text>
      </Space>
      <Table<CommandRow>
        rowKey="id"
        loading={loading}
        dataSource={rows}
        columns={[
          { title: '名称', dataIndex: 'name' },
          { title: '描述', dataIndex: 'description', ellipsis: true },
          {
            title: '命令', dataIndex: 'command', ellipsis: true,
            render: (v) => <Typography.Text code>{v}</Typography.Text>,
          },
          {
            title: '参数', key: 'params',
            render: (_, row) => {
              const ps = extractParams(row.command)
              return ps.length ? ps.map((p) => <Tag key={p}>{p}</Tag>) : '—'
            },
          },
          {
            title: '风险', dataIndex: 'danger_level',
            render: (v) => <Tag color={dangerColor[v]}>{v}</Tag>,
          },
          { title: '启用', dataIndex: 'enabled', render: (v) => <Tag color={v ? 'green' : 'default'}>{v ? '是' : '否'}</Tag> },
          {
            title: '操作', key: 'actions', width: 160,
            render: (_, row) => (
              <Space>
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
        title={editing ? `编辑命令：${editing.name}` : '新建命令'}
        open={modalOpen}
        onOk={submit}
        onCancel={() => setModalOpen(false)}
        destroyOnClose
        width={640}
      >
        <Form form={form} labelCol={{ span: 5 }} wrapperCol={{ span: 18 }}>
          <Form.Item name="name" label="名称" rules={[{ required: true }]}>
            <Input placeholder="Docker PS" />
          </Form.Item>
          <Form.Item name="description" label="描述">
            <Input placeholder="列出运行中的容器" />
          </Form.Item>
          <Form.Item name="command" label="命令" rules={[{ required: true }]}>
            <Input.TextArea rows={3} placeholder={'docker restart {{container}}'} />
          </Form.Item>
          {commandValue && extractParams(commandValue).length > 0 && (
            <Form.Item label=" " colon={false}>
              <span>参数：{extractParams(commandValue).map((p) => <Tag key={p}>{p}</Tag>)}</span>
            </Form.Item>
          )}
          <Form.Item name="danger_level" label="风险等级" initialValue="low">
            <Select options={[
              { value: 'low', label: 'low（低）' },
              { value: 'medium', label: 'medium（中）' },
              { value: 'high', label: 'high（高，需二次确认）' },
              { value: 'critical', label: 'critical（致命，需二次确认）' },
            ]} />
          </Form.Item>
          <Form.Item name="enabled" label="启用" valuePropName="checked" initialValue={true}>
            <Switch />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}
