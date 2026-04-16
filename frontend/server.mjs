import { fileURLToPath } from 'node:url'
import { dirname, join } from 'node:path'
import Fastify from 'fastify'
import fastifyStatic from '@fastify/static'
import fastifyHttpProxy from '@fastify/http-proxy'

const __dirname = dirname(fileURLToPath(import.meta.url))
const PORT = Number(process.env.PORT ?? 8080)
const HOST = process.env.HOST ?? '0.0.0.0'
const API_URL = process.env.API_URL ?? 'http://korsair-api:8090'
const DIST_DIR = join(__dirname, 'dist')

const app = Fastify({
  logger: { level: process.env.LOG_LEVEL ?? 'info' },
  disableRequestLogging: process.env.NODE_ENV === 'production',
})

await app.register(fastifyHttpProxy, {
  upstream: API_URL,
  prefix: '/api',
  rewritePrefix: '/api',
  http2: false,
})

await app.register(fastifyStatic, {
  root: DIST_DIR,
  prefix: '/',
  wildcard: false,
})

app.setNotFoundHandler((req, reply) => {
  if (req.raw.url?.startsWith('/api')) {
    return reply.code(404).send({ error: 'Not found' })
  }
  return reply.sendFile('index.html')
})

try {
  await app.listen({ port: PORT, host: HOST })
  app.log.info(`serving ${DIST_DIR} → /api proxy → ${API_URL}`)
} catch (err) {
  app.log.error(err)
  process.exit(1)
}
