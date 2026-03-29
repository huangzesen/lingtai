// 墨韵灵台配色 — ink-wash + gold lacquer (matches lingtai.ai dark mode)

// 节点状态色 (used for ACTIVE halo only)
export const inkStateColors: Record<string, string> = {
  ACTIVE:    '#7dab8f',   // 竹青
  IDLE:      '#6b8fa8',   // 苍蓝
  STUCK:     '#c4956a',   // 赭石
  ASLEEP:    '#9b8fa0',   // 藕荷
  SUSPENDED: '#b85c5c',   // 朱砂
  '':        '#4a4845',   // 淡墨
};

// 金漆 — gold lacquer palette (matching website dark mode)
export const gold = '#d4a853';       // 金
export const goldRgb = [212, 168, 83];
export const amberRgb = [196, 154, 108]; // 琥珀 — links

// 背景
export const inkBg = '#1a1a20';  // 墨黑 (matches website)

// 文字色
export const ColorText = '#e8e4df';    // 宣纸白
export const ColorTextDim = '#8a8680'; // 旧墨灰

// 边框色
export const inkBorder = '#2a2a30';  // 墨线

// 边缘色
export const inkEdgeColors = {
  avatar: '#c49a6c',  // 琥珀
  mail:   '#7dab8f',  // 竹青
};

// 向后兼容别名
export const stateColors = inkStateColors;
export const edgeColors = inkEdgeColors;
export const bg = inkBg;
