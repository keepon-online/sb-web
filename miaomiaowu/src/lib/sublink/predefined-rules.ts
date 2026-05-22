// Predefined rule categories for custom selection
// Converted from sublink-worker's UNIFIED_RULES

export interface RuleCategory {
  name: string
  label: string
  icon: string
  site_rules: string[] // GeoSite rules
  ip_rules: string[] // GeoIP rules
}

export const RULE_CATEGORIES: RuleCategory[] = [
  {
    name: 'ads',
    label: '广告拦截',
    icon: '🔴',
    site_rules: ['category-ads-all'],
    ip_rules: [],
  },
  {
    name: 'ai',
    label: 'AI 服务',
    icon: '🤖',
    site_rules: ['category-ai-!cn'],
    ip_rules: [],
  },
  {
    name: 'bilibili',
    label: '哔哩哔哩',
    icon: '📺',
    site_rules: ['bilibili'],
    ip_rules: [],
  },
  {
    name: 'youtube',
    label: '油管视频',
    icon: '📺',
    site_rules: ['youtube'],
    ip_rules: [],
  },
  {
    name: 'google',
    label: '谷歌服务',
    icon: '🔍',
    site_rules: ['google'],
    ip_rules: ['google'],
  },
  {
    name: 'private',
    label: '私有网络',
    icon: '🏠',
    site_rules: [],
    ip_rules: ['private'],
  },
  {
    name: 'domestic',
    label: '国内服务',
    icon: '🔒',
    site_rules: ['geolocation-cn', 'cn'],
    ip_rules: ['cn'],
  },
  {
    name: 'telegram',
    label: '电报消息',
    icon: '📱',
    site_rules: [],
    ip_rules: ['telegram'],
  },
  {
    name: 'github',
    label: 'Github',
    icon: '🐱',
    site_rules: ['github', 'gitlab'],
    ip_rules: [],
  },
  {
    name: 'microsoft',
    label: '微软服务',
    icon: '🪟',
    site_rules: ['microsoft'],
    ip_rules: [],
  },
  {
    name: 'apple',
    label: '苹果服务',
    icon: '🍎',
    site_rules: ['apple'],
    ip_rules: [],
  },
  {
    name: 'social',
    label: '社交媒体',
    icon: '🌐',
    site_rules: ['facebook', 'instagram', 'twitter', 'tiktok', 'linkedin'],
    ip_rules: [],
  },
  {
    name: 'streaming',
    label: '流媒体',
    icon: '📺',
    site_rules: ['netflix', 'hulu', 'disney', 'hbo', 'amazon', 'bahamut'],
    ip_rules: [],
  },
  {
    name: 'gaming',
    label: '游戏平台',
    icon: '🎮',
    site_rules: ['steam', 'epicgames', 'ea', 'ubisoft', 'blizzard'],
    ip_rules: [],
  },
  {
    name: 'education',
    label: '教育资源',
    icon: '📚',
    site_rules: [
      'coursera',
      'edx',
      'udemy',
      'khanacademy',
      'category-scholar-!cn',
    ],
    ip_rules: [],
  },
  {
    name: 'finance',
    label: '金融服务',
    icon: '💰',
    site_rules: ['paypal', 'visa', 'mastercard', 'stripe', 'wise'],
    ip_rules: [],
  },
  {
    name: 'cloud',
    label: '云服务',
    icon: '☁️',
    site_rules: ['aws', 'azure', 'digitalocean', 'heroku', 'dropbox'],
    ip_rules: [],
  },
  {
    name: 'overseas',
    label: '非中国',
    icon: '🌍',
    site_rules: ['geolocation-!cn'],
    ip_rules: [],
  },
]

/**
 * Build Clash rules from selected categories
 * Converts GeoSite and GeoIP rules to Clash format
 */
export function buildCustomRulesFromCategories(
  selectedCategories: string[]
): string[] {
  const rules: string[] = []

  for (const categoryName of selectedCategories) {
    const category = RULE_CATEGORIES.find((c) => c.name === categoryName)
    if (!category) continue

    // Add GeoSite rules (GEOSITE format for Clash)
    for (const siteRule of category.site_rules) {
      rules.push(`GEOSITE,${siteRule},PROXY`)
    }

    // Add GeoIP rules (GEOIP format for Clash)
    for (const ipRule of category.ip_rules) {
      if (ipRule === 'cn') {
        // Special handling for China IP - direct connection
        rules.push(`GEOIP,CN,DIRECT`)
      } else if (ipRule === 'private') {
        // Private network - direct connection
        rules.push(`GEOIP,PRIVATE,DIRECT`)
      } else {
        rules.push(`GEOIP,${ipRule.toUpperCase()},PROXY`)
      }
    }
  }

  // Add final fallback rule if "overseas" category is selected
  if (selectedCategories.includes('overseas')) {
    // geolocation-!cn already added as GEOSITE rule
    // Add final MATCH rule
    if (!rules.some((r) => r.startsWith('MATCH,'))) {
      rules.push('MATCH,PROXY')
    }
  }

  return rules
}

/**
 * Predefined rule set combinations
 * 注意：顺序必须与 RULE_CATEGORIES 中的定义顺序一致
 */
export const PREDEFINED_RULE_SETS = {
  // 按 RULE_CATEGORIES 顺序：ads, ai, bilibili, youtube, google, private, domestic, telegram, github...
  minimal: ['private', 'domestic', 'overseas'],
  balanced: [
    'ai',
    'youtube',
    'google',
    'private',
    'domestic',
    'telegram',
    'github',
    'overseas',
  ],
  comprehensive: RULE_CATEGORIES.map((rule) => rule.name),
}
