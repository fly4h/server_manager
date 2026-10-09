import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { Card, Col, Row, Statistic, List, Typography, Button, Spin } from 'antd'
import {
  CloudServerOutlined,
  CheckCircleOutlined,
  GlobalOutlined,
  CodeOutlined,
  ReloadOutlined,
} from '@ant-design/icons'
import { api, Stats } from '../services/api'

export default function Dashboard() {
  const [stats, setStats] = useState<Stats | null>(null)
  const [loading, setLoading] = useState(false)

  const load = async () => {
    setLoading(true)
    try {
      setStats(await api.get<Stats>('/api/dashboard/stats'))
    } catch {
      /* handled globally */ 
    } finally {
      setLoading(false)
    }
  }
  useEffect(() => { load() }, [])

  return (
    <div>
      <Row gutter={16}>
        <Col span={6}>
          <Card>
            <Statistic title="服务器" value={stats?.servers ?? '-'} prefix={<CloudServerOutlined />} />
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Statistic
              title="在线服务器"
              value={stats?.servers_online ?? '-'}
              prefix={<CheckCircleOutlined />}
              valueStyle={{ color: '#3f8600' }}
              suffix={stats ? `/ ${stats.servers_enabled}` : ''}
            />
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Statistic title="网站" value={stats?.websites ?? '-'} prefix={<GlobalOutlined />} />
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Statistic title="命令模板" value={stats?.commands ?? '-'} prefix={<CodeOutlined />} />
          </Card>
        </Col>
      </Row>

      <Card
        title="最近操作"
        style={{ marginTop: 16 }}
        extra={
          <Button icon={<ReloadOutlined />} onClick={load} loading={loading}>
            刷新
          </Button>
        }
      >
        {loading && !stats ? (
          <div style={{ textAlign: 'center', padding: 32 }}><Spin /></div>
        ) : (
          <List
            size="small"
            dataSource={stats?.recent_activity ?? []}
            locale={{ emptyText: '暂无记录' }}
            renderItem={(item) => (
              <List.Item>
                <List.Item.Meta
                  avatar={<Typography.Text type="secondary">{item.created_at}</Typography.Text>}
                  title={
                    <span>
                      <Typography.Text strong>{item.username || '-'}</Typography.Text>
                      <span style={{ margin: '0 8px' }}>·</span>
                      <Typography.Text code>{item.action}</Typography.Text>
                      {item.target_type ? (
                        <Typography.Text type="secondary"> on {item.target_type}</Typography.Text>
                      ) : null}
                    </span>
                  }
                  description={
                    <Typography.Text type="secondary" ellipsis>
                      {Object.entries(item.metadata ?? {})
                        .map(([k, v]) => `${k}=${v}`)
                        .join('  ')}
                    </Typography.Text>
                  }
                />
              </List.Item>
            )}
          />
        )}
      </Card>

      <Card title="快捷入口" style={{ marginTop: 16 }}>
        <Link to="/servers">前往服务器管理 →</Link>
        <span style={{ margin: '0 12px' }} />
        <Link to="/websites">前往网站管理 →</Link>
        <span style={{ margin: '0 12px' }} />
        <Link to="/docker">前往 Docker 管理 →</Link>
      </Card>
    </div>
  )
}
