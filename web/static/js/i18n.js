/**
 * EzPay 国际化 (i18n) 模块
 * 支持语言: en, zh-CN, zh-TW, ru, fa, vi, my
 */
(function(global) {
    'use strict';

    const I18n = {
        // 当前语言
        locale: 'zh-CN',

        // 支持的语言列表
        supportedLocales: {
            'en': { name: 'English', dir: 'ltr' },
            'zh-CN': { name: '简体中文', dir: 'ltr' },
            'zh-TW': { name: '繁體中文', dir: 'ltr' },
            'ru': { name: 'Русский', dir: 'ltr' },
            'fa': { name: 'فارسی', dir: 'rtl' },
            'vi': { name: 'Tiếng Việt', dir: 'ltr' },
            'my': { name: 'မြန်မာ', dir: 'ltr' }
        },

        // 翻译数据
        messages: {},

        // 初始化
        init: async function(defaultLocale) {
            // 优先级: URL参数 > localStorage > 浏览器语言 > 默认值
            const urlParams = new URLSearchParams(window.location.search);
            const urlLang = urlParams.get('lang');
            const storedLang = localStorage.getItem('ezpay_locale');
            const browserLang = this.detectBrowserLanguage();

            this.locale = urlLang || storedLang || browserLang || defaultLocale || 'zh-CN';

            // 确保是支持的语言
            if (!this.supportedLocales[this.locale]) {
                this.locale = 'zh-CN';
            }

            await this.loadLocale(this.locale);
            this.applyDirection();
            this.translatePage();

            return this;
        },

        // 检测浏览器语言
        detectBrowserLanguage: function() {
            const lang = navigator.language || navigator.userLanguage;

            // 精确匹配
            if (this.supportedLocales[lang]) {
                return lang;
            }

            // 前缀匹配
            const prefix = lang.split('-')[0];
            if (prefix === 'zh') {
                // 区分简繁体
                if (lang.includes('TW') || lang.includes('HK') || lang.includes('Hant')) {
                    return 'zh-TW';
                }
                return 'zh-CN';
            }

            for (const locale of Object.keys(this.supportedLocales)) {
                if (locale.startsWith(prefix)) {
                    return locale;
                }
            }

            return null;
        },

        // 加载语言文件
        loadLocale: async function(locale) {
            if (this.messages[locale]) {
                return this.messages[locale];
            }

            try {
                const response = await fetch(`/static/locales/${locale}.json`);
                if (!response.ok) {
                    throw new Error(`Failed to load locale: ${locale}`);
                }
                this.messages[locale] = await response.json();
                return this.messages[locale];
            } catch (error) {
                console.error(`Error loading locale ${locale}:`, error);
                // 回退到中文
                if (locale !== 'zh-CN') {
                    return this.loadLocale('zh-CN');
                }
                return {};
            }
        },

        // 切换语言
        setLocale: async function(locale) {
            if (!this.supportedLocales[locale]) {
                console.warn(`Unsupported locale: ${locale}`);
                return;
            }

            this.locale = locale;
            localStorage.setItem('ezpay_locale', locale);

            await this.loadLocale(locale);
            this.applyDirection();
            this.translatePage();
            this.updateLanguageSelectors();

            // 触发自定义事件
            window.dispatchEvent(new CustomEvent('localeChanged', { detail: { locale } }));
        },

        // 应用文字方向 (RTL/LTR)
        applyDirection: function() {
            const dir = this.supportedLocales[this.locale]?.dir || 'ltr';
            document.documentElement.setAttribute('dir', dir);
            document.documentElement.setAttribute('lang', this.locale);

            // 添加/移除 RTL class
            if (dir === 'rtl') {
                document.body.classList.add('rtl');
            } else {
                document.body.classList.remove('rtl');
            }
        },

        // 获取翻译
        t: function(key, params) {
            const keys = key.split('.');
            let value = this.messages[this.locale];

            for (const k of keys) {
                if (value && typeof value === 'object') {
                    value = value[k];
                } else {
                    value = undefined;
                    break;
                }
            }

            // 如果找不到，返回key本身
            if (value === undefined) {
                console.warn(`Missing translation: ${key}`);
                return key;
            }

            // 参数替换 {name} -> value
            if (params && typeof value === 'string') {
                for (const [paramKey, paramValue] of Object.entries(params)) {
                    value = value.replace(new RegExp(`\\{${paramKey}\\}`, 'g'), paramValue);
                }
            }

            return value;
        },

        // 翻译页面上所有带 data-i18n 属性的元素
        translatePage: function() {
            // 翻译文本内容
            document.querySelectorAll('[data-i18n]').forEach(el => {
                const key = el.getAttribute('data-i18n');
                if (key) {
                    el.textContent = this.t(key);
                }
            });

            // 翻译 placeholder
            document.querySelectorAll('[data-i18n-placeholder]').forEach(el => {
                const key = el.getAttribute('data-i18n-placeholder');
                if (key) {
                    el.placeholder = this.t(key);
                }
            });

            // 翻译 title
            document.querySelectorAll('[data-i18n-title]').forEach(el => {
                const key = el.getAttribute('data-i18n-title');
                if (key) {
                    el.title = this.t(key);
                }
            });

            // 翻译页面标题
            const titleEl = document.querySelector('[data-i18n-document-title]');
            if (titleEl) {
                document.title = this.t(titleEl.getAttribute('data-i18n-document-title'));
            }
        },

        // 创建语言选择器 HTML
        createLanguageSelector: function(containerId, style) {
            const container = document.getElementById(containerId);
            if (!container) return;

            const currentLang = this.supportedLocales[this.locale];
            const menuId = 'langMenu_' + containerId;

            let html = `<div class="lang-selector ${style || ''}">`;
            html += `<button class="lang-btn" onclick="I18n.toggleLangMenu('${menuId}')">`;
            html += `<span class="lang-icon">🌐</span>`;
            html += `<span class="lang-current">${currentLang.name}</span>`;
            html += `</button>`;
            html += `<div class="lang-menu" id="${menuId}">`;

            for (const [code, info] of Object.entries(this.supportedLocales)) {
                const active = code === this.locale ? 'active' : '';
                html += `<a class="lang-option ${active}" onclick="I18n.setLocale('${code}')">${info.name}</a>`;
            }

            html += `</div></div>`;

            container.innerHTML = html;
        },

        // 更新所有语言选择器的显示状态
        updateLanguageSelectors: function() {
            const currentLang = this.supportedLocales[this.locale];
            if (!currentLang) return;

            // 更新按钮上显示的语言名称
            document.querySelectorAll('.lang-current').forEach(el => {
                el.textContent = currentLang.name;
            });

            // 更新菜单项的 active 状态
            document.querySelectorAll('.lang-option').forEach(el => {
                const onclick = el.getAttribute('onclick') || '';
                const match = onclick.match(/setLocale\('([^']+)'\)/);
                if (match) {
                    if (match[1] === this.locale) {
                        el.classList.add('active');
                    } else {
                        el.classList.remove('active');
                    }
                }
            });

            // 关闭打开的菜单
            document.querySelectorAll('.lang-menu.show').forEach(m => {
                m.classList.remove('show');
            });
        },

        // 切换语言菜单显示
        toggleLangMenu: function(menuId) {
            // 先关闭所有其他菜单
            document.querySelectorAll('.lang-menu.show').forEach(m => {
                if (m.id !== menuId) m.classList.remove('show');
            });
            const menu = document.getElementById(menuId);
            if (menu) {
                menu.classList.toggle('show');
            }
        },

        // 获取当前语言信息
        getCurrentLocale: function() {
            return {
                code: this.locale,
                ...this.supportedLocales[this.locale]
            };
        },

        // 获取所有支持的语言
        getSupportedLocales: function() {
            return Object.entries(this.supportedLocales).map(([code, info]) => ({
                code,
                ...info
            }));
        }
    };

    // 点击外部关闭语言菜单
    document.addEventListener('click', function(e) {
        if (!e.target.closest('.lang-selector')) {
            document.querySelectorAll('.lang-menu.show').forEach(menu => {
                menu.classList.remove('show');
            });
        }
    });

    // 导出到全局
    global.I18n = I18n;

    // 快捷函数
    global.$t = function(key, params) {
        return I18n.t(key, params);
    };

})(window);
