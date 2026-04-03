/*
 * dmr_router.h — C header for the dmr-router static library.
 */

#ifndef DMR_ROUTER_H
#define DMR_ROUTER_H

#include <stdint.h>
#include <stddef.h>

#ifdef __cplusplus
extern "C" {
#endif

/* ── Byte slice ───────────────────────────────────────────────────────────── */

typedef struct DmrBytes {
    uint8_t *ptr;
    size_t   len;
} DmrBytes;

void dmr_free_bytes(DmrBytes b);

/* ── Streaming write callback ─────────────────────────────────────────────── *
 *
 * Rust creates an mpsc channel and passes the raw sender pointer as
 * stream_ctx.  Go calls dmr_write_chunk() for every chunk written to the
 * ResponseWriter (and on every Flush()).  Rust reads from the channel
 * receiver and forwards chunks to the axum response stream.
 *
 * After ServeHTTP returns Go calls dmr_close_stream() exactly once to
 * signal end-of-body.
 */

/*
 * Send one body chunk to the Rust stream.
 * data/len: chunk bytes.  len==0 is a flush hint.
 * Returns 0 on success, non-zero if the client disconnected.
 */
int32_t dmr_write_chunk(void *stream_ctx, const uint8_t *data, size_t len);

/*
 * Signal end-of-body and release the stream_ctx sender.
 * Must be called exactly once, after the last dmr_write_chunk.
 */
void dmr_close_stream(void *stream_ctx);

/* ── Request / response structs ───────────────────────────────────────────── */

typedef struct DmrRequest {
    const char *method;       /* NUL-terminated; valid for duration of call */
    const char *path;         /* NUL-terminated; valid for duration of call */
    DmrBytes    header_block; /* "Name: Value\0…\0"; Rust allocates, Go frees */
    DmrBytes    body;         /* request body; Rust allocates, Go frees       */
} DmrRequest;

typedef struct DmrResponse {
    uint16_t  status;
    DmrBytes  header_block;  /* "Name: Value\0…\0"; Go allocates via C malloc */
    /*
     * stream_ctx is set by Rust before calling the handler.  Go must call
     * dmr_write_chunk(stream_ctx, ...) for each body chunk and
     * dmr_close_stream(stream_ctx) when done.  Do not set body.ptr/len.
     */
    void     *stream_ctx;
} DmrResponse;

/* ── Handler callback ─────────────────────────────────────────────────────── */

typedef void (*DmrHandlerFn)(void             *ctx,
                             const DmrRequest *req,
                             DmrResponse      *resp);

/* ── Configuration ────────────────────────────────────────────────────────── */

typedef struct DmrRouterConfig {
    const char  *listen_sock;
    uint16_t     listen_port;
    DmrHandlerFn handler_fn;
    void        *handler_ctx;
    const char  *backend_sock;
    uint16_t     backend_port;
    const char  *allowed_origins;
    const char  *version;
} DmrRouterConfig;

/* ── Opaque handle ────────────────────────────────────────────────────────── */

typedef struct DmrRouterHandle DmrRouterHandle;

DmrRouterHandle *dmr_router_new_handle(void);
void             dmr_router_free_handle(DmrRouterHandle *handle);
int              dmr_router_serve(const DmrRouterConfig *cfg,
                                  DmrRouterHandle      **handle_out);
void             dmr_router_stop(DmrRouterHandle *handle);

#ifdef __cplusplus
} /* extern "C" */
#endif

#endif /* DMR_ROUTER_H */
