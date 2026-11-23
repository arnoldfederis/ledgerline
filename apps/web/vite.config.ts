import react, { reactCompilerPreset } from '@vitejs/plugin-react'
import babel from '@rolldown/plugin-babel'
import { type Connect, defineConfig, type Plugin } from 'vite'

const healthPlugin = (): Plugin => {
  const healthMiddleware: Connect.NextHandleFunction = (req, res, next) => {
    if (req.url === '/health/live' || req.url === '/health/ready') {
      const status = req.url === '/health/live' ? 'alive' : 'ready';

      res.statusCode = 200;
      res.setHeader('Content-Type', 'application/json');
      res.end(
        JSON.stringify({
          status,
          timestamp: new Date().toISOString(),
        })
      );
      return;
    }
    next();
  };

  return {
    name: 'vite-plugin-health-probes',
    configureServer(server) {
      server.middlewares.use(healthMiddleware);
    },
    configurePreviewServer(server) {
      server.middlewares.use(healthMiddleware);
    },
  };
};

// https://vite.dev/config/
export default defineConfig({
  server: {
    allowedHosts: true,
  },
  plugins: [
    react(),
    babel({presets: [reactCompilerPreset()]}),
    healthPlugin(),
  ],
})
