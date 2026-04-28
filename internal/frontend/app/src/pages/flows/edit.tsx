import { YamlEditor } from '../../components/YamlEditor';
import { renameFlow } from '../../providers/dataProvider';
import { CheckOutlined, CloseOutlined, EditOutlined, PlayCircleOutlined } from '@ant-design/icons';
import { Edit, useForm, DeleteButton } from '@refinedev/antd';
import { useNavigation, useCan } from '@refinedev/core';
import { Form, Typography, Button, Input, Space, App, Alert } from 'antd';
import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router-dom';

const { Text } = Typography;

export function FlowEdit() {
	const { t } = useTranslation();
	const { message } = App.useApp();
	const { formProps, saveButtonProps, query } = useForm({
		redirect: 'list'
	});
	const { edit, list } = useNavigation();
	const { data: canRunJobs } = useCan({ resource: 'jobs', action: 'create' });
	const [isEditing, setIsEditing] = useState(false);
	const [newFlowName, setNewFlowName] = useState('');
	const [renaming, setRenaming] = useState(false);
	const [hasChanges, setHasChanges] = useState(false);
	const [initialContent, setInitialContent] = useState('');

	const currentName = query?.data?.data?.name;

	// Track initial content
	useEffect(() => {
		if (query?.data?.data?.content && !initialContent) {
			setInitialContent(query.data.data.content);
			setHasChanges(false);
		}
	}, [query?.data?.data?.content, initialContent]);

	// Handle form value changes
	const handleValuesChange = () => {
		const currentContent = formProps.form?.getFieldValue('content');
		if (initialContent && currentContent !== undefined) {
			setHasChanges(currentContent !== initialContent);
		}
	};

	const flowNamePattern = /^[a-zA-Z0-9_-]+$/;

	const handleStartEdit = () => {
		setNewFlowName(currentName || '');
		setIsEditing(true);
	};

	const handleCancelEdit = () => {
		setIsEditing(false);
		setNewFlowName('');
	};

	const handleRename = async () => {
		// Validate new name
		if (!newFlowName) {
			message.error(t('flows.validation.newNameRequired'));
			return;
		}

		if (!flowNamePattern.test(newFlowName)) {
			message.error(t('flows.validation.newNameInvalid'));
			return;
		}

		if (newFlowName === currentName) {
			message.warning(t('flows.validation.newNameSame'));
			setIsEditing(false);
			return;
		}

		setRenaming(true);
		try {
			await renameFlow(currentName, newFlowName);
			message.success(t('flows.messages.renameSuccess'));
			setIsEditing(false);
			setNewFlowName('');
			// Navigate to the renamed flow
			edit('flows', newFlowName);
		} catch (error) {
			if (error instanceof Error) {
				if (error.message.includes('already exists')) {
					message.error(t('flows.messages.renameConflict'));
				} else if (error.message.includes('not found')) {
					message.error(t('flows.messages.renameError'));
				} else if (error.message.includes('invalid')) {
					message.error(t('flows.validation.newNameInvalid'));
				} else {
					message.error(t('flows.messages.renameError'));
				}
			} else {
				message.error(t('flows.messages.renameError'));
			}
		} finally {
			setRenaming(false);
		}
	};

	return (
		<Edit
			saveButtonProps={{ ...saveButtonProps, disabled: !hasChanges }}
			goBack={null}
			headerButtons={() => (
				<Space>
					{canRunJobs?.can && (
						<Link to={`/flows/run/${currentName}`}>
							<Button type="primary" icon={<PlayCircleOutlined />}>
								{t('flows.run')}
							</Button>
						</Link>
					)}
					<DeleteButton onSuccess={() => list('flows')} />
				</Space>
			)}>
			<div style={{ marginBottom: 24 }}>
				<div style={{ marginBottom: 8 }}>
					<Text strong>{t('common.name')}</Text>
				</div>
				{!isEditing ? (
					<Space>
						<Text style={{ fontSize: '16px' }}>{currentName}</Text>
						<Button
							type="link"
							size="small"
							icon={<EditOutlined />}
							onClick={handleStartEdit}
							style={{ padding: '0 8px' }}>
							{t('flows.actions.rename')}
						</Button>
					</Space>
				) : (
					<>
						<Space.Compact style={{ width: '100%', maxWidth: 500 }}>
							<Input
								value={newFlowName}
								onChange={(e) => setNewFlowName(e.target.value)}
								onPressEnter={handleRename}
								placeholder={t('flows.renameDialog.newNamePlaceholder')}
								disabled={renaming}
								autoFocus
								status={newFlowName && !flowNamePattern.test(newFlowName) ? 'error' : undefined}
							/>
							<Button
								type="primary"
								icon={<CheckOutlined />}
								onClick={handleRename}
								loading={renaming}
								disabled={!newFlowName || !flowNamePattern.test(newFlowName)}>
								{t('flows.renameDialog.confirm')}
							</Button>
							<Button icon={<CloseOutlined />} onClick={handleCancelEdit} disabled={renaming}>
								{t('common.cancel')}
							</Button>
						</Space.Compact>
						<Alert
							message={t('flows.renameDialog.warning')}
							type="info"
							showIcon
							style={{ marginTop: 8, maxWidth: 500 }}
							banner
						/>
					</>
				)}
			</div>

			<Form {...formProps} layout="vertical" onValuesChange={handleValuesChange}>
				<Form.Item
					label={t('flows.contentYaml')}
					name="content"
					rules={[{ required: true, message: t('flows.validation.contentRequired') }]}>
					<YamlEditor height="calc(100vh - 420px)" />
				</Form.Item>
			</Form>
		</Edit>
	);
}
