const locales: Record<string, Record<string, string>> = {
  en: {
    'connecting': 'Connecting...',
    'state.active': 'active',
    'state.idle': 'idle',
    'state.stuck': 'stuck',
    'state.asleep': 'asleep',
    'state.suspended': 'suspended',
    'mails': 'mails',
    'edge.avatar': 'avatar',
    'edge.email': 'email',
  },
  zh: {
    'connecting': '连接中...',
    'state.active': '活跃',
    'state.idle': '待命',
    'state.stuck': '卡顿',
    'state.asleep': '休眠',
    'state.suspended': '假死',
    'mails': '邮',
    'edge.avatar': '分身',
    'edge.email': '书信',
  },
  wen: {
    'connecting': '候连中...',
    'state.active': '醒',
    'state.idle': '定',
    'state.stuck': '滞',
    'state.asleep': '眠',
    'state.suspended': '假死',
    'mails': '邮',
    'edge.avatar': '分身',
    'edge.email': '书信',
  },
};

export function t(lang: string, key: string): string {
  return locales[lang]?.[key] ?? locales['en']?.[key] ?? key;
}
