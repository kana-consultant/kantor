import type { Config } from "tailwindcss";
import defaultTheme from "tailwindcss/defaultTheme";

const config: Config = {
  darkMode: ["class"],
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  theme: {
    container: {
      center: true,
      padding: "2rem",
      screens: {
        "2xl": "1400px",
      },
    },
    extend: {
      colors: {
        border: "hsl(var(--border))",
        input: "hsl(var(--input))",
        ring: "hsl(var(--ring))",
        background: "hsl(var(--background))",
        foreground: "hsl(var(--foreground))",
        module: {
          DEFAULT: "var(--module-primary)",
          light: "var(--module-light)",
          dark: "var(--module-dark)",
        },
        
        // Base Colors
        surface: {
          DEFAULT: "hsl(var(--surface))",
          muted: "hsl(var(--surface-muted))",
        },
        
        // Text Colors
        text: {
          primary: "hsl(var(--text-primary))",
          secondary: "hsl(var(--text-secondary))",
          tertiary: "hsl(var(--text-tertiary))",
        },

        primary: {
          DEFAULT: "hsl(var(--primary))",
          foreground: "hsl(var(--primary-foreground))",
        },
        secondary: {
          DEFAULT: "hsl(var(--secondary))",
          foreground: "hsl(var(--secondary-foreground))",
        },
        destructive: {
          DEFAULT: "hsl(var(--error))",
          foreground: "hsl(var(--error-foreground))",
        },
        muted: {
          DEFAULT: "hsl(var(--surface-muted))",
          foreground: "hsl(var(--text-secondary))",
        },
        accent: {
          DEFAULT: "hsl(var(--surface-muted))",
          foreground: "hsl(var(--text-primary))",
        },
        popover: {
          DEFAULT: "hsl(var(--surface))",
          foreground: "hsl(var(--text-primary))",
        },
        card: {
          DEFAULT: "hsl(var(--surface))",
          foreground: "hsl(var(--text-primary))",
        },

        // Module Identity Colors
        ops: {
          DEFAULT: "#0065FF",
          light: "#DEEBFF",
          dark: "#0747A6",
        },
        hr: {
          DEFAULT: "#6554C0",
          light: "#EAE6FF",
          dark: "#403294",
        },
        mkt: {
          DEFAULT: "#FF5630",
          light: "#FFEBE6",
          dark: "#BF2600",
        },

        // Semantic Colors
        success: {
          DEFAULT: "#36B37E",
          light: "#E3FCEF",
        },
        warning: {
          DEFAULT: "#FFAB00",
          light: "#FFFAE6",
        },
        error: {
          DEFAULT: "#FF5630",
          light: "#FFEBE6",
        },
        info: {
          DEFAULT: "#00B8D9",
          light: "#E6FCFF",
        },

        // Priority Colors
        critical: "#FF5630",
        high: "#FF8B00",
        medium: "#FFAB00",
        low: "#36B37E",

        // Pipeline Colors
        pipeline: {
          new: "#00B8D9",
          contacted: "#4C9AFF",
          qualified: "#6554C0",
          proposal: "#FF8B00",
          negotiation: "#FFAB00",
          won: "#36B37E",
          lost: "#97A0AF",
        },
        pipelineLight: {
          new: "#E6FCFF",
          contacted: "#E8F2FF",
          qualified: "#EAE6FF",
          proposal: "#FFF1E6",
          negotiation: "#FFFAE6",
          won: "#E3FCEF",
          lost: "#F4F5F7",
        },
        platform: {
          instagram: "#E4405F",
          facebook: "#1877F2",
          google: "#4285F4",
          tiktok: "#000000",
          youtube: "#FF0000",
          email: "#5E6C84",
          whatsapp: "#25D366",
          website: "#FFAB00",
          referral: "#6554C0",
          other: "#97A0AF",
        },
      },
      spacing: {
        '2': '8px',
        '3': '12px',
        '4': '16px',
        '5': '20px',
        '6': '24px',
        '8': '32px',
        '10': '40px',
        '12': '48px',
        '16': '64px',
      },
      borderRadius: {
        lg: "12px",
        md: "8px",
        sm: "6px",
        xs: "4px",
      },
      boxShadow: {
        xs: "0 1px 2px rgba(23,43,77,0.04)",
        sm: "0 1px 3px rgba(23,43,77,0.06), 0 1px 2px rgba(23,43,77,0.04)",
        md: "0 4px 8px -2px rgba(23,43,77,0.08), 0 2px 4px -2px rgba(23,43,77,0.06)",
        lg: "0 8px 16px -4px rgba(23,43,77,0.08), 0 4px 8px -4px rgba(23,43,77,0.06)",
        xl: "0 20px 32px -8px rgba(23,43,77,0.12)",
        card: "0 1px 2px rgba(23,43,77,0.04)",
        "card-hover": "0 1px 3px rgba(23,43,77,0.06), 0 1px 2px rgba(23,43,77,0.04)",
        focus: "0 0 0 2px #FFFFFF, 0 0 0 4px #4C9AFF",
      },
      fontFamily: {
        sans: ["Inter", ...defaultTheme.fontFamily.sans],
        display: ["Sora", ...defaultTheme.fontFamily.sans],
        mono: ["JetBrains Mono", ...defaultTheme.fontFamily.mono],
      },
    },
  },
  plugins: [require("tailwindcss-animate")],
};

export default config;
