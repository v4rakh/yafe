import { MinusCircleOutlined, PlusOutlined } from '@ant-design/icons';
import { Form, Input, Button, Space } from 'antd';
import { useTranslation } from 'react-i18next';

interface KeyValueInputListProps {
	name: string;
}

export function KeyValueInputList({ name }: KeyValueInputListProps) {
	const { t } = useTranslation();

	return (
		<Form.List name={name}>
			{(fields, { add, remove }) => (
				<>
					<div style={{ marginBottom: 8 }}>
						<strong>{t('form.inputsDescription')}</strong>
					</div>
					{fields.map(({ key, name: fieldName, ...restField }) => (
						<Space key={key} style={{ display: 'flex', marginBottom: 8 }} align="baseline">
							<Form.Item
								{...restField}
								name={[fieldName, 'key']}
								rules={[{ required: true, message: t('form.keyRequired') }]}>
								<Input placeholder={t('form.key')} />
							</Form.Item>
							<Form.Item
								{...restField}
								name={[fieldName, 'value']}
								rules={[{ required: true, message: t('form.valueRequired') }]}>
								<Input placeholder={t('form.value')} />
							</Form.Item>
							<MinusCircleOutlined onClick={() => remove(fieldName)} />
						</Space>
					))}
					<Form.Item>
						<Button type="dashed" onClick={() => add()} block icon={<PlusOutlined />}>
							{t('form.addInput')}
						</Button>
					</Form.Item>
				</>
			)}
		</Form.List>
	);
}
