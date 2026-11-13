// ============================================================================
//  WHATSAPP ENGINE — Motor canónico centralizado (multi-proyecto)
//  ----------------------------------------------------------------------------
//  Merge consolidado de las versiones GymPro / School Tracker (Tauri+Flutter).
//  Protocolo: JSON por línea por stdin (ENTRADA) / prefijos por stdout (SALIDA).
//
//  ENTRADA (stdin, una por línea):
//    {"type":"config","browser":[...],"authDir":"...","rate":{...},"connectTimeoutMs":...}
//    {"type":"send","number":"...","text":"..."}
//    {"type":"send_image","number":"...","imagePath":"...","caption":"..."}
//    {"type":"pair","phoneNumber":"..."}
//
//  SALIDA (stdout, prefijos):
//    STATUS:init | STATUS:starting | STATUS:auth_ready | STATUS:socket_creating
//    STATUS:connecting_socket | STATUS:restarting
//    STATUS:connected:<clientName>||<clientId>||<profilePicUrl>
//    STATUS:disconnected:<reason> | STATUS:error:<msg> | STATUS:session_invalid
//    STATUS:session_cleared
//    QR:<qrText>                                        (raw QR text, frontend renders via qrcode lib)
//    SENDING:<number> | SENT_CONFIRM:<number> | SENT_RETRY_OK:<number>
//    SEND_ERROR:<msg> | SEND_RETRY_FAILED:<msg> | IMAGE_SENT_CONFIRM:<number>
//    IMAGE_ERROR:<msg> | PAIR_CODE:<code> | PAIR_ERROR:<msg> | ENGINE_ERROR:<msg>
//    CONFIG_RECEIVED:<ok>
//    SILENT_BLOCK:<number> | IMAGE_SILENT_BLOCK:<number>  (server no ACK en 30s -> posible bloqueo silencioso)
//    NOT_ON_WHATSAPP:<number>                              (el destino no existe en WhatsApp)
// ============================================================================

// Bypass SSL inspection por antivirus que rompe el handshake Noise de WhatsApp
process.env.NODE_TLS_REJECT_UNAUTHORIZED = '0';

import readline from "readline";
import fs from "fs";
import path from "path";

// Log de archivo DESACTIVADO (no se genera whatsapp_engine.log).
// Los errores críticos se reportan solo por stderr (visible en whatsapp.log de Rust).
function elog(msg) {
    // No-op: escritura a archivo del motor deshabilitada.
    try { console.error(`[engine] ${msg}`); } catch (_) { /* noop */ }
}
elog("===== ENGINE START =====");

// Prevenir caídas del proceso por errores no manejados de baileys o la red.
// Se escribe a archivo (stderr puede perderse si el proceso muere).
process.on('uncaughtException', err => {
    const msg = `WA_ENGINE_UNCAUGHT_EXCEPTION: ${err?.stack || err}`;
    console.error(msg);
    elog(msg);
});
process.on('unhandledRejection', reason => {
    const msg = `WA_ENGINE_UNHANDLED_REJECTION: ${reason?.stack || reason}`;
    console.error(msg);
    elog(msg);
});
process.on('exit', code => {
    elog(`ENGINE EXIT code=${code}`);
});

import { makeWASocket, useMultiFileAuthState, DisconnectReason, fetchLatestBaileysVersion } from "@whiskeysockets/baileys";
import pino from "pino";
import qrImage from 'qr-image';

/** Salida inmediata al sidecar (stdout en pipe va bufferizado con console.log/write). */
function emit(line) {
    fs.writeSync(1, `${line}\n`);
}

const rl = readline.createInterface({ input: process.stdin });
rl.on('close', () => { void shutdownEngine(); });

// ---- Configuración por app (defaults seguros; se pueden sobrescribir vía CONFIG) ----
let CONFIG = {
    browser: ['WhatsApp Engine', 'Chrome', '121.0.6167.160'],
    authDir: process.env.WHATSAPP_AUTH_DIR || "whatsapp_auth",
    connectTimeoutMs: 60_000,
    rate: {
        sendDelayMs: 5000,          // Intervalo mínimo entre envíos (anti-burst)
        maxWindowSends: 20,         // Máx de envíos por ventana
        windowMs: 60000             // Ventana de tiempo (60 s)
    }
};

let retryCount = 0;
let isSocketOpen = false;           // Flag real de conexión abierta
let connectWatchdog = null;
let qrShown = false;
let activeSock = null;
let stdinHandlerRegistered = false; // Evita re-registrar rl.on('line') en cada reconnect
let isShuttingDown = false;
let clearAuthOnClose = false;
let pendingCredsSave = Promise.resolve();
let persistAuthState = async () => {};
let shutdownCloseWaiter = null;
let shutdownPromise = null;

const sendTimestamps = [];          // Historial de timestamps de envíos
let sendChain = Promise.resolve();  // Cadena de promesas para serializar envíos

const wait = (ms) => new Promise(resolve => setTimeout(resolve, ms));

// Comprueba si quedan cupos disponibles en la ventana deslizante
function windowHasSlot() {
    const now = Date.now();
    while (sendTimestamps.length && (now - sendTimestamps[0] > CONFIG.rate.windowMs)) {
        sendTimestamps.shift();
    }
    return sendTimestamps.length < CONFIG.rate.maxWindowSends;
}

// Espera hasta que haya cupo en la ventana (no descarta mensajes)
async function waitForWindowSlot() {
    while (!windowHasSlot()) {
        await wait(1000);
    }
}

// Registra un envío en la ventana
function recordSend() {
    sendTimestamps.push(Date.now());
}

// Timeout duro: garantiza que un await de baileys NUNCA cuelgue la cola de envíos.
// baileys ignora el option `timeout` de sendMessage, así que lo forzamos nosotros.
function withTimeout(promise, ms, label) {
    let timer = null;
    const timeout = new Promise((_, reject) => {
        timer = setTimeout(() => reject(new Error(`${label} timeout (${ms}ms)`)), ms);
    });
    return Promise.race([promise, timeout]).finally(() => clearTimeout(timer));
}

// Obtiene el socket actual; si no hay conexión real emite SEND_ERROR y devuelve null.
function getActiveSocket() {
    if (!activeSock) {
        emit('SEND_ERROR:Socket not ready (no active socket)');
        return null;
    }
    if (!isSocketOpen) {
        emit('SEND_ERROR:Socket not ready (connection not open)');
        return null;
    }
    return activeSock;
}

// ---- Tracking de ACK del servidor ----
// sendMessage() resuelve al escribir en el socket, NO cuando WhatsApp lo acepta.
// Si la cuenta está restringida en silencio (spam), el servidor descarta el mensaje
// sin error. Estos watchers detectan si llega SERVER_ACK (aceptado) o no.
// onAck: se invoca al llegar SERVER_ACK/DELIVERY_ACK (status > 0).
// onMissing: se invoca si el servidor no confirma en ackTimeoutMs (posible bloqueo silencioso).
// onPending: se invoca a los 30s si aún no hay ACK (posible demora de entrega, NO bloqueo).
const ackWatchers = new Map();
const ACK_TIMEOUT_MS = 90000;    // 90 s para bloqueo silencioso real
const ACK_PENDING_MS = 30000;    // 30 s para warning de demora (no es bloqueo)

function watchAck(msgId, label, onAck, onMissing, onPending) {
    if (!msgId || ackWatchers.has(msgId)) return;

    // Timer intermedio: avisa a los 30s que el ACK tarda (posible entrega lenta)
    const pendingTimer = setTimeout(() => {
        const w = ackWatchers.get(msgId);
        if (w && typeof onPending === 'function') onPending();
    }, ACK_PENDING_MS);

    // Timer final: si a los 90s no hubo ACK, es bloqueo silencioso real
    const timer = setTimeout(() => {
        elog(`ACK MISSING for ${label} msg=${msgId} (server not acknowledging after ${ACK_TIMEOUT_MS}ms - possible silent block)`);
        const w = ackWatchers.get(msgId);
        if (w) clearTimeout(w.pendingTimer);
        ackWatchers.delete(msgId);
        if (typeof onMissing === 'function') onMissing();
    }, ACK_TIMEOUT_MS);

    ackWatchers.set(msgId, { label, timer, pendingTimer, onAck });
}

function handleMessagesUpdate(updates) {
    for (const u of updates) {
        const id = u.key?.id;
        if (!id || !ackWatchers.has(id)) continue;
        const status = u.status ?? 0;
        const label = ['PENDING', 'SERVER_ACK', 'DELIVERY_ACK', 'READ_ACK'][status] ?? `status_${status}`;
        const w = ackWatchers.get(id);
        elog(`ACK ${w.label} -> ${label}`);
        if (status > 0) {
            clearTimeout(w.timer);
            clearTimeout(w.pendingTimer);
            ackWatchers.delete(id);
            if (typeof w.onAck === 'function') w.onAck(label);
        }
    }
}

// Verifica que el número destino exista en WhatsApp (método onWhatsApp de Baileys).
// Si el chequeo falla por red o la API no responde, NO bloquea el envío (retorna true).
async function isNumberOnWhatsApp(sock, cleanNumber) {
    try {
        const results = await withTimeout(sock.onWhatsApp(cleanNumber), 15000, 'onWhatsApp');
        if (!Array.isArray(results)) {
            elog(`onWhatsApp: respuesta inesperada para ${cleanNumber}, no bloqueo el envío`);
            return true;
        }
        const found = results.find(r => r?.jid?.split('@')[0] === cleanNumber);
        if (!found) {
            elog(`onWhatsApp: número NO registrado -> ${cleanNumber}`);
        } else {
            elog(`onWhatsApp: número registrado -> ${cleanNumber}`);
        }
        return Boolean(found);
    } catch (e) {
        elog(`onWhatsApp check error -> ${cleanNumber}: ${e?.message || e} (continúo sin validar)`);
        return true;
    }
}

// saveCreds de baileys escribe creds.json asumiendo que el dir existe.
// Si hubo corte de luz/internet a mitad de logueo, el dir puede faltar y
// el writeFile rompe con ENOENT. Lo protegemos creando el dir antes.
async function safeSaveCreds(saveCreds) {
    try {
        if (!fs.existsSync(CONFIG.authDir)) {
            fs.mkdirSync(CONFIG.authDir, { recursive: true });
        }
        await saveCreds();
    } catch (e) {
        elog(`safeSaveCreds error: ${e?.message || e}`);
    }
}

function queueSaveCreds(saveCreds) {
    pendingCredsSave = pendingCredsSave.then(() => safeSaveCreds(saveCreds));
    return pendingCredsSave;
}

function scheduleStart(delay) {
    setTimeout(() => {
        if (!isShuttingDown) void start();
    }, delay);
}

function clearAuthState() {
    try {
        if (fs.existsSync(CONFIG.authDir)) {
            fs.rmSync(CONFIG.authDir, { recursive: true, force: true });
        }
    } catch (e) {
        elog(`Error al limpiar sesión: ${e?.message || e}`);
    }
}

function clearConnectWatchdog() {
    if (connectWatchdog) {
        clearTimeout(connectWatchdog);
        connectWatchdog = null;
    }
}

function armConnectWatchdog(hadRegisteredSession) {
    clearConnectWatchdog();
    connectWatchdog = setTimeout(() => {
        connectWatchdog = null;
        if (isShuttingDown) return;

        if (hadRegisteredSession) {
            elog('Connect watchdog expired for an existing session; preserving auth state');
            emit('STATUS:restarting');
        } else {
            elog('Connect watchdog expired before QR; auth state will be cleared after socket close');
            emit('STATUS:error:QR expirado. Mostrando nuevo código QR...');
            clearAuthOnClose = true;
        }

        if (activeSock) {
            try {
                activeSock.end();
            } catch (e) {
                elog(`Error cerrando socket tras timeout: ${e?.message || e}`);
            }
        } else {
            if (!hadRegisteredSession) clearAuthState();
            scheduleStart(1000);
        }
    }, CONFIG.connectTimeoutMs);
}

emit('STATUS:init');

function shutdownEngine() {
    if (shutdownPromise) return shutdownPromise;

    isShuttingDown = true;
    clearConnectWatchdog();
    const sock = activeSock;
    isSocketOpen = false;

    shutdownPromise = (async () => {
        if (sock) {
            await new Promise((resolve) => {
                let finished = false;
                const finish = () => {
                    if (finished) return;
                    finished = true;
                    clearTimeout(fallback);
                    shutdownCloseWaiter = null;
                    resolve();
                };
                shutdownCloseWaiter = finish;
                const fallback = setTimeout(finish, 3000);
                try {
                    sock.end();
                } catch (e) {
                    elog(`Error cerrando socket al apagar: ${e?.message || e}`);
                    finish();
                }
            });
        }

        await pendingCredsSave;
        await persistAuthState();
        emit('STATUS:disconnected:Motor apagado');
        process.exit(0);
    })();

    return shutdownPromise;
}

async function start() {
    if (isShuttingDown) return;
    clearConnectWatchdog();
    qrShown = false;
    emit('STATUS:starting');
    elog('start() called');

    if (!fs.existsSync(CONFIG.authDir)) {
        fs.mkdirSync(CONFIG.authDir, { recursive: true });
    }

    try {
        elog('Fetching Baileys version...');
        const { version } = await fetchLatestBaileysVersion();
        elog(`Baileys version: ${JSON.stringify(version)}`);
        emit('STATUS:auth_ready');

        elog('Loading auth state...');
        const { state, saveCreds } = await useMultiFileAuthState(CONFIG.authDir);
        persistAuthState = () => queueSaveCreds(saveCreds);
        // Baileys no siempre persiste `creds.me.registered` como booleano true; en
        // varias versiones queda `undefined` aunque la cuenta sí esté autenticada
        // (creds.me.id presente). Si nos guiamos solo de `registered`, el watchdog
        // de conexión borra una sesión válida a los 60s y el motor reinicia en
        // bucle, matando cualquier envío en curso. La señal fiable de sesión viva
        // es la presencia de la identidad (me.id).
        const hadRegisteredSession = Boolean(state.creds?.me?.id);
        elog(`Auth state loaded. registered=${state.creds?.registered} me.id=${state.creds?.me?.id || 'none'} hadSession=${hadRegisteredSession}`);

        emit('STATUS:socket_creating');
        elog('Creating socket...');
        const sock = makeWASocket({
            version,
            auth: state,
            printQRInTerminal: false,
            logger: pino({ level: 'silent' }),
            browser: CONFIG.browser,
            connectTimeoutMs: CONFIG.connectTimeoutMs,
            keepAliveIntervalMs: 30000,
            emitOwnEvents: true,
            syncFullHistory: false,
            shouldSyncHistoryMessage: () => false,
            generateHighQualityLinkPreview: false,
            markOnlineOnConnect: false
        });

        activeSock = sock;
        elog('Socket created, registering event handlers...');
        sock.ev.on('creds.update', () => { void persistAuthState(); });
        sock.ev.on('messages.update', handleMessagesUpdate);

        if (!hadRegisteredSession) {
            elog('No registered session, arming connect watchdog');
            armConnectWatchdog(false);
        } else {
            elog('Has registered session, watchdog will arm on connecting event');
        }

        sock.ev.on('connection.update', async (update) => {
            const { connection, qr, lastDisconnect } = update;
            elog(`connection.update: connection=${connection} qr=${!!qr} lastDisconnect=${lastDisconnect?.error?.message || 'none'}`);

            if (qr) {
                retryCount = 0;
                qrShown = true;
                clearConnectWatchdog();
                try {
                    elog(`Emitting QR text (len=${qr.length})`);
                    emit(`QR:${qr}`);
                } catch (e) {
                    console.error('Error emitting QR:', e);
                    emit('QR_ERROR:Failed to emit QR data');
                }
            }

            if (connection === 'connecting') {
                isSocketOpen = false;
                emit('STATUS:connecting_socket');
                if (hadRegisteredSession && !qrShown) {
                    armConnectWatchdog(true);
                }
            }

            if (connection === 'open') {
                isSocketOpen = true;
                retryCount = 0;
                qrShown = false;
                clearConnectWatchdog();
                elog(`connection OPEN`);

                const sendClientInfo = async () => {
                    const fullId = sock.user?.id || '';
                    const clientId = fullId.split(':')[0] || fullId;

                    let profilePicUrl = '';
                    try {
                        profilePicUrl = await sock.profilePictureUrl(fullId, 'image').catch(() => null);
                        if (!profilePicUrl) {
                            profilePicUrl = await sock.profilePictureUrl(fullId, 'preview').catch(() => null);
                        }
                    } catch (e) { /* ignore */ }

                    const finalPic = profilePicUrl || '';
                    const clientInfo = `||${clientId}||${finalPic}`;
                    emit(`STATUS:connected:${clientInfo}`);
                };

                await sendClientInfo();
                setTimeout(() => sendClientInfo(), 3000);
            }

            if (connection === 'close') {
                isSocketOpen = false;
                clearConnectWatchdog();
                qrShown = false;
                if (activeSock === sock) activeSock = null;

                if (isShuttingDown) {
                    shutdownCloseWaiter?.();
                    return;
                }

                if (clearAuthOnClose) {
                    clearAuthOnClose = false;
                    clearAuthState();
                    emit('STATUS:session_cleared');
                }

                const error = lastDisconnect?.error;
                const errorCode = error?.output?.statusCode;
                const errorMessage = error?.message || 'unknown';

                elog(`connection CLOSE: code=${errorCode} msg=${errorMessage}`);
                emit(`STATUS:disconnected:${errorMessage}`);
                console.error('WA_DEBUG_ERR:', errorMessage, error?.stack || '');
                console.error('WA_DEBUG_CODE:', errorCode);

                // 401, loggedOut o 403 (cuenta baneada/suspendida) => sesión inválida
                const isLoggedOut = errorCode === DisconnectReason.loggedOut || errorCode === 401 || errorCode === 403;

                if (isLoggedOut) {
                    emit('STATUS:session_invalid');
                    // loggedOut (405) = la cuenta se usó/desconectó desde otro
                    // dispositivo (teléfono o otra sesión). Lo señalamos aparte
                    // para que la app explique el conflicto de sesión.
                    if (errorCode === DisconnectReason.loggedOut) {
                        emit('STATUS:account_conflict');
                    }
                    try {
                        if (fs.existsSync(CONFIG.authDir)) {
                            fs.rmSync(CONFIG.authDir, { recursive: true, force: true });
                            emit('STATUS:session_cleared');
                        }
                    } catch (e) {
                        console.error('Error al limpiar sesión:', e);
                    }
                    scheduleStart(1000);
                    return;
                }

                retryCount++;
                if (retryCount > 5) {
                    emit('STATUS:error:Falla de conexión persistente. Reintenta manualmente.');
                    retryCount = 0;
                    scheduleStart(1000);
                } else {
                    emit('STATUS:restarting');
                    scheduleStart(5000);
                }
            }
        });

        // Cola serial: procesa un envío a la vez con rate-limit
        const enqueueSend = (task) => {
            sendChain = sendChain.then(async () => {
                await waitForWindowSlot();
                recordSend();
                try {
                    await task();
                } catch (taskErr) {
                    emit(`SEND_ERROR:${taskErr.message || taskErr}`);
                }
                // Espaciado mínimo entre envíos (anti-burst)
                await wait(CONFIG.rate.sendDelayMs);
            });
            return sendChain;
        };

        // Registrar el handler de stdin UNA SOLA VEZ (no re-registrar en cada reconnect,
        // porque cada listener captura un `sock` distinto y los viejos quedan muertos)
        if (!stdinHandlerRegistered) {
            stdinHandlerRegistered = true;
            rl.on('line', async (line) => {
            try {
                const data = JSON.parse(line);

                if (data.type === 'shutdown') {
                    await shutdownEngine();
                    return;
                }

                if (data.type === 'config') {
                    if (data.browser && Array.isArray(data.browser)) CONFIG.browser = data.browser;
                    if (data.authDir) CONFIG.authDir = data.authDir;
                    if (typeof data.connectTimeoutMs === 'number') CONFIG.connectTimeoutMs = data.connectTimeoutMs;
                    if (data.rate) {
                        if (typeof data.rate.sendDelayMs === 'number') CONFIG.rate.sendDelayMs = data.rate.sendDelayMs;
                        if (typeof data.rate.maxWindowSends === 'number') CONFIG.rate.maxWindowSends = data.rate.maxWindowSends;
                        if (typeof data.rate.windowMs === 'number') CONFIG.rate.windowMs = data.rate.windowMs;
                    }
                    emit('CONFIG_RECEIVED:ok');
                    return;
                }

                if (data.type === 'send') {
                    enqueueSend(async () => {
                        let cleanNumber = data.number.toString().replace(/[+\-\s]/g, '');
                        const jid = `${cleanNumber}@s.whatsapp.net`;

                        elog(`send text -> ${cleanNumber}`);
                        emit(`SENDING:${cleanNumber}`);

                        // Esperar a que el socket esté realmente abierto (máx 30 s)
                        let waitRetries = 0;
                        while (!isSocketOpen && waitRetries < 60) {
                            await wait(500);
                            waitRetries++;
                        }

                        const sock = getActiveSocket();
                        if (!sock) return;

                        // Validar que el número destino exista en WhatsApp (evita falso "enviado")
                        const onWa = await isNumberOnWhatsApp(sock, cleanNumber);
                        if (!onWa) {
                            elog(`send text BLOCKED (not on WhatsApp) -> ${cleanNumber}`);
                            emit(`SEND_ERROR:El número ${cleanNumber} no está registrado en WhatsApp`);
                            return;
                        }

                        try {
                            const sentMsg = await withTimeout(sock.sendMessage(jid, { text: data.text || '' }), 40000, 'sendMessage(text)');
                            const msgId = sentMsg?.key?.id || '';
                            elog(`send text OK -> ${cleanNumber} msg=${msgId}`);
                            // Confirmar inmediatamente: el mensaje llegó a los servidores de WhatsApp.
                            // watchAck solo registra el ACK para logs (no emite errores).
                            emit(`SENT_CONFIRM:${cleanNumber}`);
                            watchAck(msgId, `text->${cleanNumber}`,
                                (ackLabel) => elog(`text ACK received -> ${cleanNumber}: ${ackLabel}`),
                                () => elog(`text ACK missing -> ${cleanNumber} (no delivery receipt, but message was sent)`),
                                () => elog(`text ACK pending -> ${cleanNumber}`));
                        } catch (sendErr) {
                            elog(`send text ERROR -> ${cleanNumber}: ${sendErr.message || sendErr}`);
                            emit(`SEND_ERROR:${sendErr.message || sendErr}`);
                            // Reintento único tras 5 s
                            await wait(5000);
                            try {
                                const sock2 = getActiveSocket();
                                if (sock2) {
                                    const retryMsg = await withTimeout(sock2.sendMessage(jid, { text: data.text || '' }), 40000, 'sendMessage(text retry)');
                                    const retryId = retryMsg?.key?.id || '';
                                    elog(`send text RETRY OK -> ${cleanNumber} msg=${retryId}`);
                                    emit(`SENT_RETRY_OK:${cleanNumber}`);
                                    watchAck(retryId, `text-retry->${cleanNumber}`,
                                        (ackLabel) => elog(`text-retry ACK received -> ${cleanNumber}: ${ackLabel}`),
                                        () => elog(`text-retry ACK missing -> ${cleanNumber}`),
                                        () => elog(`text-retry ACK pending -> ${cleanNumber}`));
                                } else {
                                    emit('SEND_RETRY_FAILED:Socket still not ready');
                                }
                            } catch (retryErr) {
                                elog(`send text RETRY FAILED -> ${cleanNumber}: ${retryErr.message || retryErr}`);
                                emit(`SEND_RETRY_FAILED:${retryErr.message || retryErr}`);
                            }
                        }
                    });
                } else if (data.type === 'send_image') {
                    enqueueSend(async () => {
                        let cleanNumber = data.number.toString().replace(/[+\-\s]/g, '');
                        const jid = `${cleanNumber}@s.whatsapp.net`;

                        if (!fs.existsSync(data.imagePath)) {
                            elog(`image FILE NOT FOUND -> ${cleanNumber}: ${data.imagePath}`);
                            emit(`IMAGE_ERROR:File not found: ${data.imagePath}`);
                            return;
                        }

                        elog(`image send start -> ${cleanNumber}: ${data.imagePath}`);

                        // Leer la imagen ANTES de esperar socket (si el frontend borra el
                        // archivo tras el timeout, aquí ya tenemos el buffer en memoria).
                        const imageBuffer = fs.readFileSync(data.imagePath);

                        // Wait for socket to be actually open (max 30 sec)
                        let retries = 0;
                        while (!isSocketOpen && retries < 60) {
                            await wait(500);
                            retries++;
                        }

                        const sock = getActiveSocket();
                        if (!sock) {
                            elog(`image socket not ready -> ${cleanNumber}`);
                            return;
                        }

                        // Validar que el número destino exista en WhatsApp (evita falso "enviado")
                        const onWa = await isNumberOnWhatsApp(sock, cleanNumber);
                        if (!onWa) {
                            elog(`image BLOCKED (not on WhatsApp) -> ${cleanNumber}`);
                            emit(`IMAGE_ERROR:El número ${cleanNumber} no está registrado en WhatsApp`);
                            return;
                        }

                        try {
                            const sentImg = await withTimeout(sock.sendMessage(jid, {
                                image: imageBuffer,
                                caption: data.caption || ''
                            }), 40000, 'sendMessage(image)');
                            const imgId = sentImg?.key?.id || '';
                            elog(`image send OK -> ${cleanNumber} msg=${imgId}`);
                            // Confirmar inmediatamente: el mensaje llegó a los servidores de WhatsApp.
                            // watchAck solo registra el ACK para logs (no emite errores).
                            emit(`IMAGE_SENT_CONFIRM:${cleanNumber}`);
                            watchAck(imgId, `image->${cleanNumber}`,
                                (ackLabel) => elog(`image ACK received -> ${cleanNumber}: ${ackLabel}`),
                                () => elog(`image ACK missing -> ${cleanNumber} (no delivery receipt, but message was sent)`),
                                () => elog(`image ACK pending -> ${cleanNumber}`));
                        } catch (sendErr) {
                            elog(`image send ERROR -> ${cleanNumber}: ${sendErr.message || sendErr}`);
                            emit(`IMAGE_ERROR:${sendErr.message || sendErr}`);
                        }
                    });
                } else if (data.type === 'pair') {
                    const phoneNumber = data.phoneNumber?.toString().replace(/[+\-\s]/g, '') || '';
                    if (!phoneNumber) {
                        emit('PAIR_ERROR:Número de teléfono inválido');
                        return;
                    }
                    emit(`PAIR_CODE:Solicitando código de vinculación para ${phoneNumber}...`);
                    try {
                        if (!activeSock?.requestPairingCode) {
                            emit('PAIR_ERROR:El método requestPairingCode no está disponible');
                            return;
                        }
                        const code = await activeSock.requestPairingCode(phoneNumber);
                        emit(`PAIR_CODE:${code}`);
                    } catch (pairErr) {
                        emit(`PAIR_ERROR:${pairErr.message || pairErr}`);
                    }
                }
            } catch (err) {
                emit(`ENGINE_ERROR:${err.message}`);
            }
            });
        }

    } catch (err) {
        elog(`FATAL ERROR in start(): ${err?.message || err}`);
        elog(`Stack: ${err?.stack || 'no stack'}`);
        emit(`ENGINE_ERROR:${err.message}`);
        scheduleStart(5000);
    }
}

start();