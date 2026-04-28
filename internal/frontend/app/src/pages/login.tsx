import { KeyOutlined } from '@ant-design/icons';
import { useLogin } from '@refinedev/core';
import { Form, Input, Button, Card, Typography, Alert, Space } from 'antd';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';

const { Title, Text } = Typography;

export function LoginPage() {
	const { t } = useTranslation();
	const { mutate: login, isPending } = useLogin<{ apiKey: string }>();
	const [error, setError] = useState<string | null>(null);

	const onFinish = (values: { apiKey: string }) => {
		setError(null);
		login(values, {
			onError: (err) => {
				setError(err?.message || t('auth.invalidApiKey'));
			}
		});
	};

	return (
		<div
			style={{
				height: '100vh',
				display: 'flex',
				justifyContent: 'center',
				alignItems: 'center',
				background: '#f0f2f5'
			}}>
			<Card style={{ width: 400, boxShadow: '0 4px 12px rgba(0,0,0,0.1)' }}>
				<Space direction="vertical" size="large" style={{ width: '100%' }}>
					<div style={{ textAlign: 'center' }}>
						<Title level={2} style={{ marginBottom: 8 }}>
							YaFE
						</Title>
						<Text type="secondary">Yet another Flow Engine</Text>
					</div>

					{error && <Alert message={error} type="error" showIcon closable onClose={() => setError(null)} />}

					<Form layout="vertical" onFinish={onFinish} autoComplete="off">
						<Form.Item
							name="apiKey"
							label={t('auth.apiKey')}
							rules={[{ required: true, message: t('auth.apiKeyRequired') }]}>
							<Input.Password
								prefix={<KeyOutlined />}
								placeholder={t('auth.apiKeyPlaceholder')}
								size="large"
								autoFocus
							/>
						</Form.Item>

						<Form.Item style={{ marginBottom: 0 }}>
							<Button type="primary" htmlType="submit" loading={isPending} block size="large">
								{t('auth.login')}
							</Button>
						</Form.Item>
					</Form>
				</Space>
			</Card>
		</div>
	);
}
