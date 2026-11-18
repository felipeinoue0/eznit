/// <reference types="vite/client" />

interface ImportMetaEnv {
  // The release this UI was built from (pages/About.tsx). Empty in a dev build.
  readonly VITE_APP_VERSION?: string;
}
