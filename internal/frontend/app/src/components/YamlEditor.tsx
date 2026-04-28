import { yafeCompletions } from './yafeCompletions';
import { autocompletion } from '@codemirror/autocomplete';
import { yaml } from '@codemirror/lang-yaml';
import CodeMirror from '@uiw/react-codemirror';
import { useEffect, useState } from 'react';

interface YamlEditorProps {
	value?: string;
	onChange?: (value: string) => void;
	height?: string;
}

export function YamlEditor({ value, onChange, height = '400px' }: YamlEditorProps) {
	const [theme, setTheme] = useState<'light' | 'dark'>('light');

	useEffect(() => {
		const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)');
		setTheme(mediaQuery.matches ? 'dark' : 'light');

		const handler = (e: MediaQueryListEvent) => {
			setTheme(e.matches ? 'dark' : 'light');
		};

		mediaQuery.addEventListener('change', handler);
		return () => mediaQuery.removeEventListener('change', handler);
	}, []);

	return (
		<CodeMirror
			value={value}
			height={height}
			minHeight="200px"
			theme={theme}
			extensions={[yaml(), autocompletion({ override: [yafeCompletions] })]}
			onChange={onChange}
			style={{ border: '1px solid #d9d9d9', borderRadius: 6 }}
		/>
	);
}
