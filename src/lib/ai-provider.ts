const providerLabels: Record<string, string> = {
  gemini: 'Google Gemini',
  openai: 'OpenAI',
  anthropic: 'Claude',
  claude: 'Claude',
};

export function aiProviderLabel(provider: string | undefined): string {
  return provider ? providerLabels[provider.toLowerCase()] ?? 'AI provider' : 'AI provider';
}
