// i18n.js - Lightweight internationalization for Thinkt Lite

const i18n = {
    // Current language - auto-detect from browser
    lang: navigator.language.startsWith('zh') ? 'zh' :
          navigator.language.startsWith('es') ? 'es' : 'en',

    // Language display names
    languageNames: {
        en: 'English',
        es: 'Español',
        zh: '中文'
    },

    // Translations
    translations: {
        en: {
            // Navigation
            'app-title': 'thinkt lite',
            'api-docs': 'API Docs',
            'api-sources': 'API: Sources',
            'api-projects': 'API: Projects',
            'api-apps': 'API: Apps',
            'api-themes': 'API: Themes',

            // Panels
            'sources': 'Sources',
            'apps': 'Apps',
            'themes': 'Themes',
            'projects': 'Projects',

            // Loading & Empty states
            'loading': 'Loading...',
            'no-sources': 'No sources found',
            'no-apps': 'No apps configured',
            'no-themes': 'No themes found',
            'no-projects': 'No projects found',
            'no-visible-projects': 'No visible projects',

            // Modal
            'api-response': 'API Response',
            'view-raw': 'View Raw',
            'copy-url': 'Copy URL',
            'close': 'Close',
            'copied': 'Copied!',

            // Status
            'ok': 'OK',
            'na': 'N/A',
            'ready': 'Ready',
            'active': 'active',

            // Theme metadata
            'built-in-theme': 'Built-in theme',
            'user-theme': 'User theme',
            'built-in': 'built-in',
            'user': 'user',

            // Project metadata
            'sessions': 'sessions',
            'session': 'session',
            'click-to-copy': 'Click to copy path',

            // Toggle visibility
            'toggle-visibility': 'Toggle visibility',

            // Path status
            'path-missing': 'Folder does not exist',

            // Time formatting
            'today': 'Today',
            'yesterday': 'Yesterday',
            'days-ago': '{n}d ago',

            // Error
            'error': 'Error',

            // Connection status
            'connected': 'Connected',
            'disconnected': 'Disconnected',
            'checking': 'Checking...',

            // Open in
            'open-in': 'Open in',
        },

        es: {
            // Navegación
            'app-title': 'thinkt lite',
            'api-docs': 'Docs API',
            'api-sources': 'API: Fuentes',
            'api-projects': 'API: Proyectos',
            'api-apps': 'API: Apps',
            'api-themes': 'API: Temas',

            // Paneles
            'sources': 'Fuentes',
            'apps': 'Apps',
            'themes': 'Temas',
            'projects': 'Proyectos',

            // Carga y estados vacíos
            'loading': 'Cargando...',
            'no-sources': 'No se encontraron fuentes',
            'no-apps': 'No hay apps configuradas',
            'no-themes': 'No se encontraron temas',
            'no-projects': 'No se encontraron proyectos',
            'no-visible-projects': 'No hay proyectos visibles',

            // Modal
            'api-response': 'Respuesta API',
            'view-raw': 'Ver Raw',
            'copy-url': 'Copiar URL',
            'close': 'Cerrar',
            'copied': '¡Copiado!',

            // Estado
            'ok': 'OK',
            'na': 'N/D',
            'ready': 'Listo',
            'active': 'activo',

            // Metadatos de tema
            'built-in-theme': 'Tema incorporado',
            'user-theme': 'Tema de usuario',
            'built-in': 'incorporado',
            'user': 'usuario',

            // Metadatos de proyecto
            'sessions': 'sesiones',
            'session': 'sesión',
            'click-to-copy': 'Clic para copiar ruta',

            // Alternar visibilidad
            'toggle-visibility': 'Alternar visibilidad',

            // Estado de ruta
            'path-missing': 'La carpeta no existe',

            // Formato de tiempo
            'today': 'Hoy',
            'yesterday': 'Ayer',
            'days-ago': 'hace {n}d',

            // Error
            'error': 'Error',

            // Connection status
            'connected': 'Conectado',
            'disconnected': 'Desconectado',
            'checking': 'Comprobando...',

            // Abrir en
            'open-in': 'Abrir en',
        },

        zh: {
            // 导航
            'app-title': 'thinkt lite',
            'api-docs': 'API 文档',
            'api-sources': 'API: 来源',
            'api-projects': 'API: 项目',
            'api-apps': 'API: 应用',
            'api-themes': 'API: 主题',

            // 面板
            'sources': '来源',
            'apps': '应用',
            'themes': '主题',
            'projects': '项目',

            // 加载和空状态
            'loading': '加载中...',
            'no-sources': '未找到来源',
            'no-apps': '未配置应用',
            'no-themes': '未找到主题',
            'no-projects': '未找到项目',
            'no-visible-projects': '无可见项目',

            // 弹窗
            'api-response': 'API 响应',
            'view-raw': '查看原始',
            'copy-url': '复制链接',
            'close': '关闭',
            'copied': '已复制!',

            // 状态
            'ok': '正常',
            'na': '不可用',
            'ready': '就绪',
            'active': '激活',

            // 主题元数据
            'built-in-theme': '内置主题',
            'user-theme': '用户主题',
            'built-in': '内置',
            'user': '用户',

            // 项目元数据
            'sessions': '会话',
            'session': '会话',
            'click-to-copy': '点击复制路径',

            // 切换可见性
            'toggle-visibility': '切换可见性',

            // 路径状态
            'path-missing': '文件夹不存在',

            // 时间格式
            'today': '今天',
            'yesterday': '昨天',
            'days-ago': '{n}天前',

            // 错误
            'error': '错误',

            // 连接状态
            'connected': '已连接',
            'disconnected': '已断开',
            'checking': '检查中...',

            // 打开方式
            'open-in': '打开方式',
        }
    },

    // Get translation
    t(key, params = {}) {
        const text = this.translations[this.lang]?.[key] || 
                     this.translations.en[key] || 
                     key;
        return text.replace(/\{(\w+)\}/g, (_, k) => params[k] ?? `{${k}}`);
    },

    // Set language
    setLanguage(lang) {
        if (this.translations[lang]) {
            this.lang = lang;
            document.documentElement.lang = lang;
            // Update static text content for elements with data-i18n
            document.querySelectorAll('[data-i18n]').forEach(el => {
                const key = el.getAttribute('data-i18n');
                if (el.hasAttribute('title')) {
                    el.title = this.t(key);
                } else {
                    el.textContent = this.t(key);
                }
            });
        }
    },

    // Get available languages
    getLanguages() {
        return Object.keys(this.translations);
    }
};

// Helper function for templates
function t(key, params) {
    return i18n.t(key, params);
}

// Global setLanguage function for onclick handlers
function setLanguage(lang) {
    i18n.setLanguage(lang);
    updateLangSelector();
    // Reload dynamic content
    if (typeof loadSources === 'function') loadSources();
    if (typeof loadApps === 'function') loadApps();
    if (typeof loadThemes === 'function') loadThemes();
    if (typeof loadProjects === 'function') loadProjects();
}

// Toggle language dropdown menu
function toggleLangMenu() {
    const selector = document.getElementById('lang-selector');
    selector.classList.toggle('open');
}

// Close language menu when clicking outside
document.addEventListener('click', (e) => {
    const selector = document.getElementById('lang-selector');
    if (selector && !selector.contains(e.target)) {
        selector.classList.remove('open');
    }
});

// Update language selector
function updateLangSelector() {
    const currentEl = document.querySelector('.lang-current');
    const menuEl = document.getElementById('lang-menu');

    if (currentEl) {
        currentEl.textContent = i18n.languageNames[i18n.lang] || i18n.lang.toUpperCase();
    }

    if (menuEl) {
        const languages = i18n.getLanguages();
        menuEl.innerHTML = languages.map(lang => `
            <button class="lang-menu-item ${lang === i18n.lang ? 'active' : ''}"
                    onclick="selectLanguage('${lang}')">
                <span class="lang-check">✓</span>${i18n.languageNames[lang] || lang.toUpperCase()}
            </button>
        `).join('');
    }
}

// Select language from dropdown
function selectLanguage(lang) {
    setLanguage(lang);
    document.getElementById('lang-selector').classList.remove('open');
}

// Initialize on page load
document.addEventListener('DOMContentLoaded', () => {
    // Set document language
    document.documentElement.lang = i18n.lang;

    // Update all static elements with data-i18n attribute
    document.querySelectorAll('[data-i18n]').forEach(el => {
        const key = el.getAttribute('data-i18n');
        if (el.tagName === 'BUTTON' || el.tagName === 'A') {
            // Preserve any child elements, just update text
            const text = i18n.t(key);
            if (el.textContent.trim()) {
                el.textContent = text;
            }
        } else {
            el.textContent = i18n.t(key);
        }
    });

    // Build and set initial language selector state
    updateLangSelector();
});

// Close dropdown on Escape key
document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape') {
        const selector = document.getElementById('lang-selector');
        if (selector) selector.classList.remove('open');
    }
});
