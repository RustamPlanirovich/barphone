import 'dart:async';
import 'dart:io';

import 'package:app_links/app_links.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:wakelock_plus/wakelock_plus.dart';

import 'protocol/client.dart';
import 'protocol/models.dart';
import 'state.dart';
import 'ui/add_machine.dart';
import 'ui/home.dart';
import 'ui/theme.dart';

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  // A deck lives on screen: full screen, never dims.
  await SystemChrome.setEnabledSystemUIMode(SystemUiMode.immersiveSticky);
  unawaited(WakelockPlus.enable());
  final app = AppState();
  // Icons come over HTTPS from agents with self-signed certificates: accept exactly the
  // ones the paired computers are pinned to. Set before anything opens a connection.
  HttpOverrides.global = PinningOverrides(() => {for (final l in app.machines) ?l.machine.fp});
  await app.init();
  runApp(BarphoneApp(app: app));
}

class BarphoneApp extends StatefulWidget {
  const BarphoneApp({super.key, required this.app});
  final AppState app;

  @override
  State<BarphoneApp> createState() => _BarphoneAppState();
}

class _BarphoneAppState extends State<BarphoneApp> with WidgetsBindingObserver {
  final _navigator = GlobalKey<NavigatorState>();
  StreamSubscription<Uri>? _links;
  final _handledCodes = <String>{};

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    // barphone://pair?... from the system camera app or `adb shell am start -d`.
    _links = AppLinks().uriLinkStream.listen(_onLink);
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    if (state == AppLifecycleState.resumed) {
      SystemChrome.setEnabledSystemUIMode(SystemUiMode.immersiveSticky);
      WakelockPlus.enable();
    }
  }

  Future<void> _onLink(Uri uri) async {
    final p = PairUri.parse(uri.toString());
    if (p == null || !_handledCodes.add('${p.machineId}/${p.code}')) return;
    // Wait for the first frame so a navigator exists.
    await WidgetsBinding.instance.endOfFrame;
    final ctx = _navigator.currentContext;
    if (ctx == null || !ctx.mounted) return;
    _navigator.currentState?.popUntil((r) => r.isFirst);
    await runPairing(ctx, p.name, () => widget.app.pairWithUri(p));
  }

  @override
  void dispose() {
    _links?.cancel();
    WidgetsBinding.instance.removeObserver(this);
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'barphone',
      debugShowCheckedModeBanner: false,
      navigatorKey: _navigator,
      theme: buildTheme(),
      home: HomeScreen(app: widget.app),
    );
  }
}
