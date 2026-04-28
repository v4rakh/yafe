import { useList } from '@refinedev/core';
import { Button, Tooltip } from 'antd';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router-dom';

interface FlowLinkProps {
	flowName: string;
}

interface Flow {
	id: string;
	name: string;
}

export function FlowLink({ flowName }: FlowLinkProps) {
	const { t } = useTranslation();
	const { result } = useList<Flow>({
		resource: 'flows',
		pagination: { mode: 'off' }
	});

	const flows = result?.data ?? [];
	const flowExists = flows.some((f) => f.name === flowName);

	if (flowExists) {
		return (
			<Link to={`/flows/edit/${flowName}`}>
				<Button type="link" size="small" style={{ padding: 0 }}>
					{flowName}
				</Button>
			</Link>
		);
	}

	return (
		<Tooltip title={t('flows.flowNoLongerExists')}>
			<Button type="link" size="small" style={{ padding: 0 }} disabled>
				{flowName}
			</Button>
		</Tooltip>
	);
}
