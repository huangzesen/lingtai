// 墨韵灵台配色 — 画师·悟空

// 节点状态色
export const inkStateColors: Record<string, string> = {
  ACTIVE:    '#7dab8f',   // 竹青
  IDLE:      '#8ab4c4',   // 藤紫（苍蓝）
  STUCK:     '#c9743a',   // 赭石
  ASLEEP:    '#d4a5c9',   // 藕荷
  SUSPENDED: '#c46b6b',   // 朱砂
  '':        '#4a5568',   // 淡墨
};

// 节点类型色（覆盖状态色的类型区分）
export const inkNodeTypeColors = {
  orchestrator: '#c4946c',  // 琥珀（器灵主）
  human:        '#e8e4df',  // 宣纸白
  avatar:       '#7dab8f',  // 竹青
};

// 边缘色
export const inkEdgeColors = {
  avatar: '#7dab8f',  // 竹青实线
  mail:  '#8ab4c4',  // 藤紫（苍蓝）虚线
};

// 背景
export const inkBg = '#0d0d0f';  // 墨黑

// 文字色（Go 端 ColorText / ColorTextDim）
export const ColorText = '#e8e4df';    // 宣纸白
export const ColorTextDim = '#8a8680'; // 旧墨灰

// 边框色
export const inkBorder = '#2a2a30';  // 墨线

// 向后兼容别名
export const stateColors = inkStateColors;
export const edgeColors = inkEdgeColors;
export const bg = inkBg;
