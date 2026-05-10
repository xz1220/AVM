import {defineConfig} from "tsup";

export default defineConfig({
  entry: {
    "avm-ui": "src/main.tsx"
  },
  outDir: "dist",
  outExtension: () => ({js: ".js"}),
  clean: true,
  format: ["esm"],
  platform: "node",
  target: "node22",
  splitting: false,
  noExternal: ["commander", "fuse.js", "ink", "ink-text-input", "react", "zod"],
  banner: {
    js: 'import {createRequire} from "node:module"; const require = createRequire(import.meta.url);'
  },
  sourcemap: false,
  minify: false
});
