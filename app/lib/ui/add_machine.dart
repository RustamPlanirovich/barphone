import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:mobile_scanner/mobile_scanner.dart';

import '../protocol/models.dart';
import '../state.dart';
import 'theme.dart';

/// Shows a progress dialog while [job] runs; returns true on success, shows the error otherwise.
Future<bool> runPairing(BuildContext context, String name, Future<String?> Function() job) async {
  showDialog<void>(
    context: context,
    barrierDismissible: false,
    builder: (_) => PopScope(
      canPop: false,
      child: AlertDialog(
        content: Row(
          children: [
            const SizedBox.square(dimension: 28, child: CircularProgressIndicator(strokeWidth: 3, color: C.accent)),
            const SizedBox(width: 18),
            Expanded(child: Text('Подключаюсь к «$name»…')),
          ],
        ),
      ),
    ),
  );
  final error = await job();
  if (!context.mounted) return error == null;
  Navigator.of(context, rootNavigator: true).pop();
  if (error == null) {
    HapticFeedback.mediumImpact();
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('«$name» подключён'), duration: const Duration(seconds: 2)));
    return true;
  }
  HapticFeedback.heavyImpact();
  await showDialog<void>(
    context: context,
    builder: (d) => AlertDialog(
      title: const Text('Не удалось подключиться'),
      content: Text(error),
      actions: [TextButton(onPressed: () => Navigator.pop(d), child: const Text('Понятно'))],
    ),
  );
  return false;
}

class AddMachineScreen extends StatefulWidget {
  const AddMachineScreen({super.key, required this.app});
  final AppState app;

  @override
  State<AddMachineScreen> createState() => _AddMachineScreenState();
}

class _AddMachineScreenState extends State<AddMachineScreen> {
  final _scanner = MobileScannerController(formats: const [BarcodeFormat.qrCode]);
  bool _busy = false;

  @override
  void dispose() {
    _scanner.dispose();
    super.dispose();
  }

  Future<void> _done(Future<bool> pairing) async {
    if (await pairing && mounted) Navigator.pop(context);
  }

  Future<void> _onDetect(BarcodeCapture capture) async {
    if (_busy) return;
    for (final code in capture.barcodes) {
      final p = PairUri.parse(code.rawValue ?? '');
      if (p == null) continue;
      _busy = true;
      await _scanner.stop();
      if (!mounted) return;
      final ok = await runPairing(context, p.name, () => widget.app.pairWithUri(p));
      if (ok) {
        if (mounted) Navigator.pop(context);
        return;
      }
      _busy = false;
      if (mounted) await _scanner.start();
      return;
    }
  }

  Future<void> _pairDiscovered(DiscoveredAgent a) async {
    final code = await _askCode(context, a.name);
    if (code == null || !mounted) return;
    await _done(runPairing(context, a.name, () => widget.app.pairDiscovered(a, code)));
  }

  Future<void> _manual() async {
    final res = await showDialog<(String, String)>(context: context, builder: (_) => const _ManualDialog());
    if (res == null || !mounted) return;
    await _done(runPairing(context, res.$1, () => widget.app.pairManual(res.$1, res.$2)));
  }

  @override
  Widget build(BuildContext context) {
    return ListenableBuilder(
      listenable: widget.app,
      builder: (context, _) {
        final found = widget.app.discovered;
        return Scaffold(
          appBar: AppBar(title: const Text('Добавить компьютер'), backgroundColor: C.bg),
          body: ListView(
            padding: const EdgeInsets.fromLTRB(20, 4, 20, 24),
            children: [
              const Text(
                'На компьютере: значок barphone в трее → «Подключить телефон». Наведите камеру на QR-код.',
                style: TextStyle(color: C.muted, height: 1.35),
              ),
              const SizedBox(height: 16),
              ClipRRect(
                borderRadius: BorderRadius.circular(20),
                child: AspectRatio(
                  aspectRatio: 1,
                  // The scanner widget keeps showing its placeholder when permission is denied
                  // before it initialized, so errors are rendered from the controller state.
                  child: ValueListenableBuilder<MobileScannerState>(
                    valueListenable: _scanner,
                    builder: (context, v, scanner) => v.error != null
                        ? _cameraError(v.error!)
                        : Stack(
                            fit: StackFit.expand,
                            children: [
                              scanner!,
                              IgnorePointer(
                                child: Center(
                                  child: FractionallySizedBox(
                                    widthFactor: .62,
                                    heightFactor: .62,
                                    child: DecoratedBox(
                                      decoration: BoxDecoration(
                                        border: Border.all(color: C.accent, width: 3),
                                        borderRadius: BorderRadius.circular(18),
                                      ),
                                    ),
                                  ),
                                ),
                              ),
                            ],
                          ),
                    child: MobileScanner(
                      controller: _scanner,
                      onDetect: _onDetect,
                      placeholderBuilder: (_) => const _CameraMessage('Запуск камеры…'),
                      errorBuilder: (_, e) => _cameraError(e),
                    ),
                  ),
                ),
              ),
              const SizedBox(height: 24),
              Row(
                children: [
                  const Text('Найдены в сети', style: TextStyle(fontSize: 16, fontWeight: FontWeight.w600)),
                  const SizedBox(width: 10),
                  if (found.isEmpty) const SizedBox.square(dimension: 16, child: CircularProgressIndicator(strokeWidth: 2, color: C.muted)),
                ],
              ),
              const SizedBox(height: 8),
              if (found.isEmpty)
                const Text(
                  'Ищу компьютеры с barphone… Если ничего не находится, используйте QR или ввод адреса.',
                  style: TextStyle(color: C.muted, fontSize: 13),
                ),
              for (final a in found)
                Card(
                  color: C.tile,
                  margin: const EdgeInsets.only(bottom: 8),
                  child: ListTile(
                    leading: Icon(a.os == 'macos' ? Icons.laptop_mac_rounded : Icons.desktop_windows_rounded),
                    title: Text(a.name),
                    subtitle: Text(widget.app.isPaired(a.id) ? 'уже подключён · ${a.hosts.first}' : a.hosts.first),
                    trailing: const Icon(Icons.chevron_right_rounded),
                    onTap: () => _pairDiscovered(a),
                  ),
                ),
              const SizedBox(height: 12),
              TextButton.icon(
                onPressed: _manual,
                icon: const Icon(Icons.keyboard_rounded),
                label: const Text('Ввести адрес и код вручную'),
              ),
            ],
          ),
        );
      },
    );
  }
}

Widget _cameraError(MobileScannerException e) => _CameraMessage(
  e.errorCode == MobileScannerErrorCode.permissionDenied
      ? 'Нет доступа к камере. Разрешите его в настройках Android или подключитесь по коду ниже.'
      : 'Камера недоступна. Подключитесь по коду ниже.',
);

class _CameraMessage extends StatelessWidget {
  const _CameraMessage(this.text);
  final String text;

  @override
  Widget build(BuildContext context) => Container(
    color: C.surface,
    padding: const EdgeInsets.all(28),
    alignment: Alignment.center,
    child: Column(
      mainAxisSize: MainAxisSize.min,
      children: [
        const Icon(Icons.no_photography_outlined, size: 36, color: C.muted),
        const SizedBox(height: 12),
        Text(
          text,
          textAlign: TextAlign.center,
          style: const TextStyle(color: C.muted, height: 1.35),
        ),
      ],
    ),
  );
}

Future<String?> _askCode(BuildContext context, String name) {
  final ctl = TextEditingController();
  return showDialog<String>(
    context: context,
    builder: (d) => AlertDialog(
      title: Text('Код для «$name»'),
      content: TextField(
        controller: ctl,
        autofocus: true,
        keyboardType: TextInputType.number,
        maxLength: 6,
        inputFormatters: [FilteringTextInputFormatter.digitsOnly],
        style: const TextStyle(fontSize: 28, letterSpacing: 8, fontWeight: FontWeight.w700),
        textAlign: TextAlign.center,
        decoration: const InputDecoration(hintText: '000000', counterText: ''),
        onSubmitted: (v) => Navigator.pop(d, v),
      ),
      actions: [
        TextButton(onPressed: () => Navigator.pop(d), child: const Text('Отмена')),
        FilledButton(onPressed: () => Navigator.pop(d, ctl.text), child: const Text('Подключить')),
      ],
    ),
  );
}

class _ManualDialog extends StatefulWidget {
  const _ManualDialog();
  @override
  State<_ManualDialog> createState() => _ManualDialogState();
}

class _ManualDialogState extends State<_ManualDialog> {
  final _addr = TextEditingController();
  final _code = TextEditingController();

  @override
  Widget build(BuildContext context) {
    return AlertDialog(
      title: const Text('Адрес компьютера'),
      content: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          TextField(
            controller: _addr,
            autofocus: true,
            keyboardType: const TextInputType.numberWithOptions(decimal: true),
            decoration: const InputDecoration(labelText: 'IP-адрес', hintText: 'например, 192.168.0.10'),
          ),
          const SizedBox(height: 12),
          TextField(
            controller: _code,
            keyboardType: TextInputType.number,
            maxLength: 6,
            inputFormatters: [FilteringTextInputFormatter.digitsOnly],
            decoration: const InputDecoration(labelText: 'Код с экрана компьютера', counterText: ''),
          ),
          const SizedBox(height: 8),
          const Text('Адрес и код показаны в окне «Подключить телефон» на компьютере.', style: TextStyle(color: C.muted, fontSize: 12)),
        ],
      ),
      actions: [
        TextButton(onPressed: () => Navigator.pop(context), child: const Text('Отмена')),
        FilledButton(
          onPressed: () {
            if (_addr.text.trim().isEmpty || _code.text.length != 6) return;
            Navigator.pop(context, (_addr.text.trim(), _code.text));
          },
          child: const Text('Подключить'),
        ),
      ],
    );
  }
}
