import adapter from '@sveltejs/adapter-static';

/** @type {import('@sveltejs/kit').Config} */
const config = {
  kit: {
    adapter: adapter({
      out: 'build',
      precompress: false,
      fallback: 'index.html', // may differ from host to host
      polyfill: true,
      strict: false,
    })
  },
};

export default config;
