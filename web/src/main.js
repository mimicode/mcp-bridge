import { computed, createApp, h, onBeforeUnmount, onMounted, ref, watch } from "vue";
import {
  darkTheme,
  NConfigProvider,
  NDialogProvider,
  NMessageProvider,
  NNotificationProvider
} from "naive-ui";
import App from "./App.vue";

const app = createApp({
  setup() {
    const storageKey = "mcp-bridge-admin-theme";
    const mediaQuery = typeof window !== "undefined" ? window.matchMedia("(prefers-color-scheme: dark)") : null;
    const systemPrefersDark = ref(mediaQuery?.matches ?? false);
    const themePreference = ref(readThemePreference(storageKey));
    const resolvedTheme = computed(() => {
      if (themePreference.value === "system") {
        return systemPrefersDark.value ? "dark" : "light";
      }
      return themePreference.value;
    });
    const naiveTheme = computed(() => (resolvedTheme.value === "dark" ? darkTheme : null));

    const syncSystemPreference = event => {
      systemPrefersDark.value = event.matches;
    };

    onMounted(() => {
      mediaQuery?.addEventListener?.("change", syncSystemPreference);
    });

    onBeforeUnmount(() => {
      mediaQuery?.removeEventListener?.("change", syncSystemPreference);
    });

    watch(
      themePreference,
      value => {
        if (typeof window !== "undefined") {
          window.localStorage.setItem(storageKey, value);
        }
      },
      { immediate: true }
    );

    watch(
      resolvedTheme,
      value => {
        if (typeof document !== "undefined") {
          document.documentElement.style.colorScheme = value;
        }
      },
      { immediate: true }
    );

    return () =>
      h(NConfigProvider, { theme: naiveTheme.value }, {
        default: () =>
          h(NDialogProvider, null, {
            default: () =>
              h(NNotificationProvider, null, {
                default: () =>
                  h(NMessageProvider, null, {
                    default: () =>
                      h(App, {
                        themePreference: themePreference.value,
                        resolvedTheme: resolvedTheme.value,
                        "onUpdate:themePreference": value => {
                          themePreference.value = value;
                        }
                      })
                  })
              })
          })
      });
  }
});

app.mount("#app");

function readThemePreference(storageKey) {
  if (typeof window === "undefined") {
    return "light";
  }
  const value = window.localStorage.getItem(storageKey);
  if (value === "light" || value === "dark" || value === "system") {
    return value;
  }
  return "light";
}
