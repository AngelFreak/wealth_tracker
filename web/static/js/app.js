// Wealth Tracker - Main JavaScript
// Uses Alpine.js for reactivity and HTMX for server communication

document.addEventListener('alpine:init', () => {
    // Theme store - manages dark/light mode
    Alpine.store('theme', {
        dark: localStorage.getItem('theme') !== 'light',

        init() {
            this.apply();
        },

        toggle() {
            this.dark = !this.dark;
            this.apply();
            this.save();
        },

        apply() {
            if (this.dark) {
                document.documentElement.classList.add('dark');
            } else {
                document.documentElement.classList.remove('dark');
            }
        },

        save() {
            localStorage.setItem('theme', this.dark ? 'dark' : 'light');
            // Also save to server if logged in
            if (document.body.dataset.authenticated === 'true') {
                fetch('/settings/theme', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json',
                    },
                    body: JSON.stringify({ theme: this.dark ? 'dark' : 'light' }),
                });
            }
        }
    });

    // Toast notifications
    Alpine.store('toast', {
        message: '',
        type: 'success',
        visible: false,
        timeout: null,

        show(message, type = 'success', duration = 3000) {
            this.message = message;
            this.type = type;
            this.visible = true;

            if (this.timeout) {
                clearTimeout(this.timeout);
            }

            this.timeout = setTimeout(() => {
                this.visible = false;
            }, duration);
        },

        success(message) {
            this.show(message, 'success');
        },

        error(message) {
            this.show(message, 'error');
        }
    });

    // Confirmation modal store
    Alpine.store('confirm', {
        visible: false,
        title: '',
        message: '',
        type: 'danger', // 'danger' or 'warning'
        confirmText: 'Confirm',
        callback: null,
        formToSubmit: null,

        // Show confirmation dialog
        show(options) {
            this.title = options.title || 'Are you sure?';
            this.message = options.message || 'This action cannot be undone.';
            this.type = options.type || 'danger';
            this.confirmText = options.confirmText || 'Confirm';
            this.callback = options.onConfirm || null;
            this.formToSubmit = options.form || null;
            this.visible = true;
        },

        // Execute the confirmed action
        execute() {
            if (this.formToSubmit) {
                this.formToSubmit.submit();
            } else if (this.callback) {
                this.callback();
            }
            this.cancel();
        },

        // Cancel and close
        cancel() {
            this.visible = false;
            this.callback = null;
            this.formToSubmit = null;
        }
    });
});

// Initialize theme immediately to prevent flash
(function() {
    if (localStorage.getItem('theme') !== 'light') {
        document.documentElement.classList.add('dark');
    }
})();

// HTMX event handlers
document.addEventListener('htmx:afterSwap', (event) => {
    // Re-initialize any Alpine components in swapped content
    if (typeof Alpine !== 'undefined') {
        Alpine.initTree(event.detail.target);
    }
});

document.addEventListener('htmx:responseError', (event) => {
    // Show error toast on failed requests
    const message = event.detail.xhr.responseText || 'An error occurred';
    if (typeof Alpine !== 'undefined' && Alpine.store('toast')) {
        Alpine.store('toast').error(message);
    }
});

// Format currency values
function formatCurrency(amount, currency = 'DKK') {
    return new Intl.NumberFormat('da-DK', {
        style: 'decimal',
        minimumFractionDigits: 2,
        maximumFractionDigits: 2,
    }).format(amount) + ' ' + currency.toLowerCase() + '.';
}

// Format percentage
function formatPercent(value) {
    const prefix = value >= 0 ? '+' : '';
    return prefix + value.toFixed(2) + '%';
}

// Number input formatting with thousand separators
// Make globally available for inline scripts
window.NumberFormat = {
    // Locale mapping from user preference to JS locale string
    locales: {
        'da': 'da-DK',
        'en': 'en-US',
        'fr': 'fr-FR',
        'de': 'de-DE'
    },

    // Get locale from document or default to da-DK
    getLocale() {
        const format = document.body.dataset.numberFormat || 'da';
        return this.locales[format] || 'da-DK';
    },

    // Get decimal separator for locale
    getDecimalSeparator(locale) {
        return (1.1).toLocaleString(locale).charAt(1);
    },

    // Get thousand separator for locale
    getThousandSeparator(locale) {
        return (1000).toLocaleString(locale).charAt(1);
    },

    // Format number with thousand separators
    format(value, decimals = 0) {
        const locale = this.getLocale();
        const num = parseFloat(value);
        if (isNaN(num)) return '';

        return num.toLocaleString(locale, {
            minimumFractionDigits: decimals,
            maximumFractionDigits: decimals
        });
    },

    // Parse formatted string back to number
    parse(value) {
        if (!value) return 0;
        const locale = this.getLocale();
        const decimalSep = this.getDecimalSeparator(locale);
        const thousandSep = this.getThousandSeparator(locale);

        // Remove thousand separators and replace decimal separator with dot
        let cleaned = value.toString();
        if (thousandSep) {
            cleaned = cleaned.split(thousandSep).join('');
        }
        if (decimalSep !== '.') {
            cleaned = cleaned.replace(decimalSep, '.');
        }

        const num = parseFloat(cleaned);
        return isNaN(num) ? 0 : num;
    },

    // Initialize a single input with formatting
    initInput(input) {
        const decimals = parseInt(input.dataset.decimals || '0');
        const hiddenInput = input.nextElementSibling;
        const locale = this.getLocale();
        const decimalSep = this.getDecimalSeparator(locale);
        const thousandSep = this.getThousandSeparator(locale);

        // Format initial value if present
        if (hiddenInput && hiddenInput.value) {
            input.value = this.format(hiddenInput.value, decimals);
        }

        // Handle input events
        input.addEventListener('input', (e) => {
            const cursorPos = e.target.selectionStart;
            const oldLength = e.target.value.length;

            // Get raw value (digits, decimal separator, minus)
            let raw = e.target.value;

            // Allow only digits, decimal separator, and minus
            const allowedChars = new RegExp(`[^0-9${decimalSep === '.' ? '\\.' : decimalSep}-]`, 'g');
            raw = raw.replace(allowedChars, '');

            // Parse to number
            const numValue = this.parse(raw);

            // Update hidden input
            if (hiddenInput) {
                hiddenInput.value = numValue || '';
            }

            // Format for display (only if we have a value)
            if (raw && numValue !== 0) {
                // Check if user is typing decimals
                const hasDecimal = raw.includes(decimalSep);
                const decimalPart = hasDecimal ? raw.split(decimalSep)[1] || '' : '';
                const displayDecimals = hasDecimal ? Math.min(decimalPart.length, decimals) : 0;

                e.target.value = this.format(numValue, displayDecimals);

                // Adjust cursor position
                const newLength = e.target.value.length;
                const diff = newLength - oldLength;
                e.target.setSelectionRange(cursorPos + diff, cursorPos + diff);
            } else if (!raw) {
                e.target.value = '';
                if (hiddenInput) hiddenInput.value = '';
            }
        });

        // Format on blur with full decimals
        input.addEventListener('blur', (e) => {
            const numValue = this.parse(e.target.value);
            if (numValue !== 0) {
                e.target.value = this.format(numValue, decimals);
            }
            if (hiddenInput) {
                hiddenInput.value = numValue || '';
            }
        });

        // Handle paste
        input.addEventListener('paste', (e) => {
            e.preventDefault();
            const text = e.clipboardData.getData('text');
            const numValue = this.parse(text);
            if (numValue !== 0) {
                e.target.value = this.format(numValue, decimals);
                if (hiddenInput) hiddenInput.value = numValue;
            }
        });
    },

    // Initialize all number inputs on page
    init() {
        document.querySelectorAll('[data-format-number]').forEach(input => {
            this.initInput(input);
        });
    }
};

// Initialize number inputs when DOM is ready
document.addEventListener('DOMContentLoaded', () => {
    NumberFormat.init();
});

// Re-initialize after HTMX swaps
document.addEventListener('htmx:afterSwap', () => {
    NumberFormat.init();
});

// Format date
function formatDate(dateStr) {
    const date = new Date(dateStr);
    return date.toLocaleDateString('da-DK', {
        year: 'numeric',
        month: 'short',
        day: 'numeric',
    });
}

// Chart.js default configuration for dark mode
if (typeof Chart !== 'undefined') {
    const isDark = () => document.documentElement.classList.contains('dark');

    Chart.defaults.color = isDark() ? '#a1a1aa' : '#6b7280';
    Chart.defaults.borderColor = isDark() ? '#262626' : '#e5e7eb';
    Chart.defaults.font.family = "'Inter', sans-serif";

    // Update chart colors when theme changes
    document.addEventListener('alpine:initialized', () => {
        Alpine.effect(() => {
            const dark = Alpine.store('theme').dark;
            Chart.defaults.color = dark ? '#a1a1aa' : '#6b7280';
            Chart.defaults.borderColor = dark ? '#262626' : '#e5e7eb';

            // Update all existing charts
            Object.values(Chart.instances).forEach(chart => {
                chart.update();
            });
        });
    });
}
