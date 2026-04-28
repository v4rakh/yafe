import { CompletionContext, CompletionResult } from '@codemirror/autocomplete';

interface CompletionItem {
	label: string;
	detail?: string;
	info?: string;
}

const topLevelCompletions: CompletionItem[] = [
	{ label: 'runs-on', detail: 'required', info: "Platform to run the flow on (currently only 'host')" },
	{ label: 'steps', detail: 'required', info: 'List of steps to execute' },
	{ label: 'state-dir', info: 'Directory for persistent state between runs' },
	{ label: 'secrets', info: 'Secret declarations for the flow' }
];

const stepCompletions: CompletionItem[] = [
	{ label: 'kind', detail: 'required', info: "Step type (currently only 'shell')" },
	{ label: 'cmd', detail: 'required', info: 'Command to execute' },
	{ label: 'name', info: 'Step identifier for referencing outputs' },
	{ label: 'shell', info: 'Shell to use: bash, sh, or zsh' },
	{ label: 'env', info: 'Environment variables for this step' },
	{ label: 'outputs', info: 'Output declarations for this step' },
	{ label: 'secrets', info: 'Secrets to inject into this step' }
];

const outputCompletions: CompletionItem[] = [
	{ label: 'name', info: 'Output identifier for referencing' },
	{ label: 'type', info: "Output type: 'variable' or 'file'" },
	{ label: 'path', info: 'File path for file-type outputs' }
];

const secretDeclarationCompletions: CompletionItem[] = [
	{ label: 'name', info: 'Secret identifier for referencing' },
	{ label: 'from', info: 'Secret source configuration' }
];

const secretSourceCompletions: CompletionItem[] = [
	{ label: 'env', info: 'Environment variable containing the secret' },
	{ label: 'file', info: 'File path containing the secret' }
];

const valueCompletions: Record<string, CompletionItem[]> = {
	'runs-on': [{ label: 'host', info: 'Run on the host machine' }],
	kind: [{ label: 'shell', info: 'Execute a shell command' }],
	shell: [
		{ label: 'bash', info: 'Bash shell' },
		{ label: 'sh', info: 'POSIX shell' },
		{ label: 'zsh', info: 'Z shell' }
	],
	type: [
		{ label: 'variable', info: 'Capture command output as variable' },
		{ label: 'file', info: 'Reference an output file' }
	]
};

function getIndentLevel(line: string): number {
	const match = line.match(/^(\s*)/);
	return match ? match[1].length : 0;
}

function getContext(
	doc: string,
	pos: number
): 'top' | 'step' | 'output' | 'secret-declaration' | 'secret-source' | 'value' {
	const lines = doc.slice(0, pos).split('\n');
	const currentLine = lines[lines.length - 1];
	const currentIndent = getIndentLevel(currentLine);

	// Check if we're completing a value (after a colon)
	if (currentLine.includes(':')) {
		const keyMatch = currentLine.match(/^\s*(\w[\w-]*)\s*:\s*/);
		if (keyMatch && valueCompletions[keyMatch[1]]) {
			return 'value';
		}
	}

	// Walk backwards to find context
	let inSteps = false;
	let inOutputs = false;
	let inSecrets = false;
	let inFrom = false;
	let stepsIndent = -1;
	let outputsIndent = -1;
	let secretsIndent = -1;
	let fromIndent = -1;

	for (let i = lines.length - 2; i >= 0; i--) {
		const line = lines[i];
		const indent = getIndentLevel(line);
		const trimmed = line.trim();

		if (trimmed.startsWith('from:') && fromIndent === -1) {
			fromIndent = indent;
			if (currentIndent > indent) {
				inFrom = true;
			}
		}

		if (trimmed.startsWith('outputs:') && outputsIndent === -1) {
			outputsIndent = indent;
			if (currentIndent > indent && !inFrom) {
				inOutputs = true;
			}
		}

		if (trimmed.startsWith('secrets:') && secretsIndent === -1) {
			secretsIndent = indent;
			if (currentIndent > indent && !inFrom && !inOutputs) {
				inSecrets = true;
			}
		}

		if (trimmed.startsWith('steps:') && stepsIndent === -1) {
			stepsIndent = indent;
			if (currentIndent > indent && !inOutputs && !inSecrets && !inFrom) {
				inSteps = true;
			}
		}

		// If we hit a line with less indent than current, stop searching
		if (indent < currentIndent && trimmed !== '') {
			break;
		}
	}

	if (inFrom) return 'secret-source';
	if (inOutputs) return 'output';
	if (inSecrets) return 'secret-declaration';
	if (inSteps) return 'step';
	return 'top';
}

function getValueKey(line: string): string | null {
	const match = line.match(/^\s*(\w[\w-]*)\s*:\s*/);
	return match ? match[1] : null;
}

export function yafeCompletions(context: CompletionContext): CompletionResult | null {
	const word = context.matchBefore(/[\w-]*/);
	if (!word) return null;

	const doc = context.state.doc.toString();
	const line = context.state.doc.lineAt(context.pos);
	const lineText = line.text;

	const ctx = getContext(doc, context.pos);

	let completions: CompletionItem[];

	if (ctx === 'value') {
		const key = getValueKey(lineText);
		if (key && valueCompletions[key]) {
			completions = valueCompletions[key];
		} else {
			return null;
		}
	} else {
		switch (ctx) {
			case 'step':
				completions = stepCompletions;
				break;
			case 'output':
				completions = outputCompletions;
				break;
			case 'secret-declaration':
				completions = secretDeclarationCompletions;
				break;
			case 'secret-source':
				completions = secretSourceCompletions;
				break;
			default:
				completions = topLevelCompletions;
		}
	}

	return {
		from: word.from,
		options: completions.map((c) => ({
			label: c.label,
			detail: c.detail,
			info: c.info,
			type: ctx === 'value' ? 'constant' : 'property'
		}))
	};
}
