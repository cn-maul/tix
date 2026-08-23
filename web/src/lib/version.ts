declare const __APP_VERSION__: string

// 应用版本号（构建时由 vite.config.ts 从 package.json 注入）
export const APP_VERSION = __APP_VERSION__

export const APP_VERSION_LABEL = `v${APP_VERSION}`