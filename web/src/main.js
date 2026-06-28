import { createApp, h } from "vue";
import {
  createDiscreteApi,
  darkTheme,
  NConfigProvider,
  NDialogProvider,
  NMessageProvider,
  NNotificationProvider
} from "naive-ui";
import App from "./App.vue";

const app = createApp({
  setup() {
    return () =>
      h(NConfigProvider, { theme: darkTheme }, {
        default: () =>
          h(NDialogProvider, null, {
            default: () =>
              h(NNotificationProvider, null, {
                default: () =>
                  h(NMessageProvider, null, {
                    default: () => h(App)
                  })
              })
          })
      });
  }
});

app.mount("#app");

export const discrete = createDiscreteApi(["message", "dialog", "notification"], {
  configProviderProps: {
    theme: darkTheme
  }
});
