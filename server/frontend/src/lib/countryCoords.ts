/**
 * 地图视图坐标解析（完全离线，无 GeoIP 依赖）
 *
 * 优先级：region 城市关键词 → country_code 国家中心
 * 坐标格式 [经度, 纬度]，与 ECharts geo 坐标系一致
 */

/** 国家代码 → 国家中心坐标 [lon, lat] */
export const COUNTRY_COORDS: Record<string, [number, number]> = {
  CN: [104.2, 35.0],
  HK: [114.17, 22.32],
  MO: [113.55, 22.2],
  TW: [121.0, 23.7],
  JP: [138.25, 36.2],
  KR: [127.77, 35.91],
  KP: [127.0, 40.0],
  SG: [103.82, 1.35],
  MY: [102.0, 4.21],
  TH: [100.99, 15.12],
  VN: [106.3, 16.6],
  PH: [122.9, 11.8],
  ID: [113.92, -2.55],
  IN: [78.96, 21.0],
  PK: [69.35, 30.38],
  BD: [90.36, 23.68],
  LK: [80.77, 7.87],
  NP: [84.12, 28.39],
  KZ: [66.92, 48.02],
  UZ: [64.59, 41.38],
  RU: [95.0, 61.52],
  BY: [27.95, 53.71],
  UA: [31.17, 48.38],
  MD: [28.47, 47.41],
  TR: [35.24, 38.96],
  IL: [34.85, 31.04],
  AE: [54.0, 24.0],
  SA: [45.08, 23.89],
  QA: [51.18, 25.35],
  KW: [47.48, 29.31],
  BH: [50.55, 26.07],
  OM: [55.98, 21.47],
  JO: [36.24, 30.06],
  LB: [35.86, 33.85],
  IQ: [43.68, 33.22],
  IR: [53.69, 32.43],
  AF: [66.4, 33.94],
  US: [-98.35, 39.5],
  CA: [-106.35, 56.13],
  MX: [-102.55, 23.63],
  BR: [-51.93, -14.24],
  AR: [-63.62, -38.42],
  CL: [-71.54, -35.68],
  CO: [-74.3, 4.57],
  PE: [-75.02, -9.19],
  UY: [-55.77, -32.52],
  GB: [-1.17, 52.36],
  IE: [-8.0, 53.41],
  FR: [2.21, 46.23],
  DE: [10.45, 51.17],
  NL: [5.29, 52.13],
  BE: [4.47, 50.5],
  LU: [6.13, 49.61],
  CH: [8.23, 46.8],
  AT: [14.14, 47.52],
  IT: [12.57, 41.87],
  ES: [-3.75, 40.46],
  PT: [-8.0, 39.4],
  DK: [9.5, 56.26],
  NO: [10.75, 59.91],
  SE: [15.76, 59.33],
  FI: [25.75, 61.92],
  IS: [-18.64, 65.0],
  PL: [19.15, 51.92],
  CZ: [15.47, 49.82],
  SK: [17.11, 48.67],
  HU: [19.5, 47.16],
  RO: [24.97, 45.94],
  BG: [25.23, 42.73],
  GR: [21.82, 39.07],
  HR: [16.37, 45.1],
  SI: [14.99, 46.15],
  RS: [20.45, 44.02],
  BA: [17.68, 43.92],
  MK: [21.75, 41.61],
  AL: [20.17, 41.15],
  LT: [23.88, 55.17],
  LV: [24.6, 56.88],
  EE: [25.76, 58.68],
  ZA: [24.75, -29.0],
  EG: [30.8, 26.82],
  NG: [8.68, 9.08],
  KE: [37.91, -0.02],
  MA: [-7.09, 31.79],
  DZ: [1.66, 28.03],
  TN: [9.6, 33.89],
  AU: [134.49, -25.73],
  NZ: [172.84, -41.5],
  FJ: [178.07, -17.71],
}

/** 城市关键词 → 城市坐标 [lon, lat]（region 字段优先匹配，用于国家级定位的细化） */
const CITY_KEYWORDS: Array<{ keywords: string[]; coord: [number, number] }> = [
  // 中国城市
  { keywords: ['上海', 'shanghai'], coord: [121.47, 31.23] },
  { keywords: ['北京', 'beijing'], coord: [116.4, 39.9] },
  { keywords: ['广州', 'guangzhou'], coord: [113.26, 23.13] },
  { keywords: ['深圳', 'shenzhen'], coord: [114.06, 22.54] },
  { keywords: ['杭州', 'hangzhou'], coord: [120.16, 30.29] },
  { keywords: ['成都', 'chengdu'], coord: [104.07, 30.57] },
  { keywords: ['重庆', 'chongqing'], coord: [106.55, 29.56] },
  { keywords: ['青岛', 'qingdao'], coord: [120.38, 36.07] },
  { keywords: ['张家口', 'zhangjiakou'], coord: [114.88, 40.82] },
  // 香港城市级
  { keywords: ['香港', 'hongkong', 'hong kong', 'hk'], coord: [114.17, 22.32] },
  { keywords: ['台北', 'taipei'], coord: [121.56, 25.03] },
  // 日本
  { keywords: ['东京', 'tokyo'], coord: [139.69, 35.69] },
  { keywords: ['大阪', 'osaka'], coord: [135.5, 34.69] },
  { keywords: ['名古屋', 'nagoya'], coord: [136.91, 35.17] },
  // 韩国
  { keywords: ['首尔', 'seoul'], coord: [126.98, 37.57] },
  // 东南亚
  { keywords: ['新加坡', 'singapore'], coord: [103.82, 1.35] },
  { keywords: ['曼谷', 'bangkok'], coord: [100.5, 13.76] },
  { keywords: ['雅加达', 'jakarta'], coord: [106.85, -6.21] },
  { keywords: ['马尼拉', 'manila'], coord: [120.98, 14.6] },
  { keywords: ['吉隆坡', 'kuala lumpur'], coord: [101.69, 3.14] },
  { keywords: ['胡志明', 'ho chi minh'], coord: [106.63, 10.82] },
  { keywords: ['河内', 'hanoi'], coord: [105.83, 21.03] },
  // 美国
  { keywords: ['洛杉矶', 'los angeles', 'lax'], coord: [-118.24, 34.05] },
  { keywords: ['圣何塞', 'san jose', 'sjc'], coord: [-121.89, 37.34] },
  { keywords: ['硅谷', 'silicon valley'], coord: [-122.08, 37.39] },
  { keywords: ['西雅图', 'seattle'], coord: [-122.33, 47.61] },
  { keywords: ['纽约', 'new york', 'nyc'], coord: [-74.01, 40.71] },
  { keywords: ['达拉斯', 'dallas'], coord: [-96.8, 32.78] },
  { keywords: ['芝加哥', 'chicago'], coord: [-87.63, 41.88] },
  { keywords: ['迈阿密', 'miami'], coord: [-80.19, 25.76] },
  { keywords: ['凤凰城', 'phoenix', 'phx'], coord: [-112.07, 33.45] },
  { keywords: ['盐湖城', 'salt lake city'], coord: [-111.89, 40.76] },
  { keywords: ['丹佛', 'denver'], coord: [-104.99, 39.74] },
  { keywords: ['阿什本', 'ashburn'], coord: [-77.49, 39.04] },
  // 加拿大
  { keywords: ['多伦多', 'toronto'], coord: [-79.38, 43.65] },
  { keywords: ['温哥华', 'vancouver'], coord: [-123.12, 49.28] },
  { keywords: ['蒙特利尔', 'montreal'], coord: [-73.57, 45.5] },
  // 欧洲
  { keywords: ['伦敦', 'london'], coord: [-0.13, 51.51] },
  { keywords: ['法兰克福', 'frankfurt'], coord: [8.68, 50.11] },
  { keywords: ['阿姆斯特丹', 'amsterdam'], coord: [4.9, 52.37] },
  { keywords: ['巴黎', 'paris'], coord: [2.35, 48.86] },
  { keywords: ['卢森堡', 'luxembourg'], coord: [6.13, 49.61] },
  { keywords: ['苏黎世', 'zurich'], coord: [8.54, 47.38] },
  { keywords: ['维也纳', 'vienna'], coord: [16.37, 48.21] },
  { keywords: ['布拉格', 'prague'], coord: [14.44, 50.08] },
  { keywords: ['华沙', 'warsaw'], coord: [21.01, 52.23] },
  { keywords: ['莫斯科', 'moscow'], coord: [37.62, 55.76] },
  { keywords: ['马德里', 'madrid'], coord: [-3.7, 40.42] },
  { keywords: ['米兰', 'milan'], coord: [9.19, 45.46] },
  { keywords: ['斯德哥尔摩', 'stockholm'], coord: [18.07, 59.33] },
  { keywords: ['赫尔辛基', 'helsinki'], coord: [24.94, 60.17] },
  { keywords: ['雷克雅未克', 'reykjavik'], coord: [-21.9, 64.15] },
  // 中东
  { keywords: ['迪拜', 'dubai'], coord: [55.27, 25.2] },
  { keywords: ['特拉维夫', 'tel aviv'], coord: [34.78, 32.08] },
  { keywords: ['伊斯坦布尔', 'istanbul'], coord: [28.98, 41.01] },
  // 其他
  { keywords: ['悉尼', 'sydney'], coord: [151.21, -33.87] },
  { keywords: ['墨尔本', 'melbourne'], coord: [144.96, -37.81] },
  { keywords: ['奥克兰', 'auckland'], coord: [174.76, -36.85] },
  { keywords: ['圣保罗', 'sao paulo'], coord: [-46.63, -23.55] },
  { keywords: ['孟买', 'mumbai'], coord: [72.88, 19.08] },
]

/**
 * 解析服务器坐标：region 城市关键词优先，否则 country_code 国家中心
 * 返回 null 表示无法定位（不显示在地图上）
 */
export function resolveServerCoord(
  region: string | undefined,
  countryCode: string,
): [number, number] | null {
  const text = (region || '').toLowerCase()
  if (text) {
    for (const { keywords, coord } of CITY_KEYWORDS) {
      for (const kw of keywords) {
        if (text.includes(kw)) return coord
      }
    }
  }
  return COUNTRY_COORDS[(countryCode || '').toUpperCase()] || null
}
