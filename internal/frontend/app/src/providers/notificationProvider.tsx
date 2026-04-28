import type { NotificationProvider } from '@refinedev/core';
import { App } from 'antd';

export function useAppNotificationProvider(): NotificationProvider {
	const { message } = App.useApp();

	return {
		open: (params) => {
			const { message: msg, description, type } = params;
			// Use description if available, otherwise use message
			const content = description || msg;

			// Don't show empty messages
			if (!content) return;

			// Show message with appropriate type (3 seconds for success/info, 4 for errors)
			const messageType: string = type || 'info';
			const duration = messageType === 'error' ? 4 : 3;

			switch (messageType) {
				case 'success':
					message.success(content, duration);
					break;
				case 'error':
					message.error(content, duration);
					break;
				case 'warning':
					message.warning(content, duration);
					break;
				default:
					// 'info' or 'progress'
					message.info(content, duration);
					break;
			}
		},
		close: () => {
			// Close all messages
			message.destroy();
		}
	};
}
