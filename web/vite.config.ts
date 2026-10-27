import { sveltekit } from '@sveltejs/kit/vite';
import adapter from '@sveltejs/adapter-static';
import { defineConfig } from 'vite';
export default defineConfig({
  plugins: [
    sveltekit({ adapter: adapter({ pages: 'build', assets: 'build', fallback: 'index.html' }) })
  ],
  server: {
    allowedHosts: ['terminal.local'],
    proxy: { '/api': { target: 'http://127.0.0.1:8080' } }
  }
});
