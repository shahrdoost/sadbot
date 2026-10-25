import { vi } from 'vitest'

// Node's Request rejects the relative URLs that fetchBaseQuery produces in
// jsdom; resolve them against a fixed origin for the duration of each test.
export function stubFetch(handler: (input: string, init?: RequestInit) => Response | Promise<Response>) {
  const RealRequest = globalThis.Request
  class PatchedRequest extends RealRequest {
    constructor(input: RequestInfo | URL, init?: RequestInit) {
      if (typeof input === 'string' && input.startsWith('/')) {
        input = `http://localhost${input}`
      }
      super(input, init)
    }
  }
  vi.stubGlobal('Request', PatchedRequest)
  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      let url: string
      let effectiveInit = init
      if (input instanceof Request) {
        url = input.url
        // fetchBaseQuery passes a Request object; recover the body for handlers
        const body = await input.clone().text()
        effectiveInit = { ...init, method: input.method, body: body || init?.body, headers: input.headers as never }
      } else {
        url = typeof input === 'string' ? input : String(input)
      }
      return handler(url, effectiveInit)
    }),
  )
}

export function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
}

