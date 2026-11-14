// ============================================================================
//  INTEGRACIÓN FLUTTER (Dart) — WhatsApp Engine centralizado
//  ----------------------------------------------------------------------------
//  El motor canónico vive en: D:\Mis Proyectos\WhatsApp Engine
//  Empaquetar para Flutter:  node bundle.mjs --single   (UN solo index.js)
//  Copiar index.js a:  whatsapp_engine/index.js  (carpeta junto al ejecutable)
//  La sesión se guarda en WHATSAPP_AUTH_DIR (evita "logged in elsewhere").
// ============================================================================

import 'dart:io';
import 'dart:convert';
import 'package:path_provider/path_provider.dart';

class WhatsappService {
  Process? _process;
  String? _enginePath;

  Future<void> init() async {
    if (_enginePath != null) return;
    // Search order: 1) cwd/whatsapp_engine (dev) 2) executable dir/.. 3) app docs
    final candidates = <String>[
      '${Directory.current.path}/whatsapp_engine',
      '${Platform.resolvedExecutable}/../whatsapp_engine',
      '${(await getApplicationDocumentsDirectory()).path}/whatsapp_engine',
    ];
    for (final c in candidates) {
      final normal = c.replaceAll('\\', '/');
      if (await File('$normal/index.js').exists()) {
        _enginePath = normal;
        break;
      }
    }
    _enginePath ??= '${(await getApplicationDocumentsDirectory()).path}/whatsapp_engine';
    await Directory('$_enginePath').create(recursive: true);
  }

  // Encuentra node.exe (Windows busca en rutas típicas; macro usa which)
  Future<String> _findNode() async {
    try {
      final r = await Process.run('where', ['node']); if (r.exitCode == 0) return 'node';
    } catch (_) {}
    for (final p in [
      r'C:\Program Files\nodejs\node.exe',
      r'C:\Program Files (x86)\nodejs\node.exe',
      r'C:\nvm\node.exe',
    ]) {
      if (await File(p).exists()) return p;
    }
    throw Exception('Node.js no está instalado');
  }

  Future<void> start() async {
    await init();
    if (_process != null) return;

    final engineJs = '$_enginePath/index.js';
    final nodeBin = await _findNode();

    // Definir el directorio de sesión ÚNICO de este proyecto
    final authDir = '$_enginePath/whatsapp_auth';

    _process = await Process.start(nodeBin, [engineJs], environment: {
      'WHATSAPP_AUTH_DIR': authDir,
    });

    _process!.stdout.transform(utf8.decoder).transform(const LineSplitter()).listen((line) {
      // Parsear eventos
      if (line.startsWith('QR_DATA_URL:')) {
        final url = line.substring('QR_DATA_URL:'.length);
        // <- mostrar url como QR en FlutterWidget (qr_flutter)
      } else if (line.startsWith('STATUS:connected:')) {
        _onConnected(line.substring('STATUS:connected:'.length));
      } else if (line.startsWith('STATUS:session_invalid')) {
        _onSessionInvalid();
      } else if (line.startsWith('STATUS:disconnected:')) {
        // desconectado
      } else if (line.startsWith('PAIR_CODE:')) {
        // código de vinculación
      } else if (line.startsWith('SENT_CONFIRM:') || line.startsWith('SENT_RETRY_OK:')) {
        _onSent();
      }
    });

    _process!.stderr.listen((d) => stdout.writeln('[WA] ${utf8.decode(d, allowMalformed: true)}'));
  }

  // Enviar configuración (opcional — tiene defaults seguros)
  void sendConfig({List<String>? browser, int? sendDelayMs, int? maxWindowSends, int? windowMs}) {
    _process?.stdin.writeln(jsonEncode({
      'type': 'config',
      if (browser != null) 'browser': browser,
      if (sendDelayMs != null || maxWindowSends != null || windowMs != null)
        'rate': { if (sendDelayMs != null) 'sendDelayMs': sendDelayMs,
                  if (maxWindowSends != null) 'maxWindowSends': maxWindowSends,
                  if (windowMs != null) 'windowMs': windowMs },
    }));
  }

  void sendText(String number, String text) {
    _process?.stdin.writeln(jsonEncode({'type': 'send', 'number': number, 'text': text}));
  }

  void sendImage(String number, String imagePath, [String caption = '']) {
    _process?.stdin.writeln(jsonEncode(
        {'type': 'send_image', 'number': number, 'imagePath': imagePath, 'caption': caption}));
  }

  void pair(String phoneNumber) {
    _process?.stdin.writeln(jsonEncode({'type': 'pair', 'phoneNumber': phoneNumber}));
  }

  void stop() {
    _process?.kill();
    _process = null;
  }

  // callbacks que debes implementar
  void _onClientInfo(String info) {}
  void _onSent() {}
  void _onSessionInvalid() {}
}