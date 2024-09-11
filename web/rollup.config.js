import replace from '@rollup/plugin-replace';
import resolve from '@rollup/plugin-node-resolve';
import terser from '@rollup/plugin-terser';
import typescript from '@rollup/plugin-typescript';

export default {
  input: 'src/main.ts',
  output: {
    name: 'app',
    file: 'static/app.js',
    format: 'iife',
    sourcemap: true,
  },
  plugins: [
    typescript(),
    resolve(),
    replace({
      preventAssignment: false,
      'Reflect.decorate': 'undefined',
      'SERVICE_BASE_URL': process.env.SERVICE_BASE_URL ?? '',
    }),
    terser({
      ecma: 2021,
      module: true,
      warnings: true,
      format: {
        comments: false,
      },
      mangle: {
        properties: {
          regex: /^__/,
        },
      },
    }),
  ],
};
