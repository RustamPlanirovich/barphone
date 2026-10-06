import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../protocol/link.dart';
import '../protocol/models.dart';
import 'theme.dart';

/// Controls of a Google Meet call on the PC, in the second half of a landscape screen
/// when only one computer is connected. Whether the microphone is on cannot be known
/// (Meet keeps it open while muted), so that button is a plain toggle; the camera is.
class CallPanel extends StatelessWidget {
  const CallPanel({super.key, required this.link});
  final MachineLink link;

  Future<void> _do(BuildContext context, String action) async {
    HapticFeedback.mediumImpact();
    final r = await link.callAction(action);
    if (r.ok || !context.mounted) return;
    final why = switch (r.error) {
      'not_found' => 'Окно Meet не найдено — звонок закончился или вкладка Meet не на переднем плане в браузере',
      'unsupported' => 'Управление звонком выключено в настройке barphone на компьютере',
      _ => launchError(r.error),
    };
    ScaffoldMessenger.of(context)
      ..hideCurrentSnackBar()
      ..showSnackBar(SnackBar(content: Text(why), duration: const Duration(seconds: 3)));
  }

  @override
  Widget build(BuildContext context) {
    return ValueListenableBuilder<CallInfo?>(
      valueListenable: link.call,
      builder: (context, call, _) {
        final camera = call?.camera ?? false;
        return Column(
          children: [
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 10, 12, 4),
              child: Row(
                children: [
                  const Icon(Icons.video_call_rounded, color: C.ok, size: 22),
                  const SizedBox(width: 10),
                  Expanded(
                    child: Text(
                      'Google Meet · ${link.machine.name}',
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: const TextStyle(fontSize: 17, fontWeight: FontWeight.w600),
                    ),
                  ),
                ],
              ),
            ),
            Expanded(
              child: Padding(
                padding: const EdgeInsets.all(12),
                child: Column(
                  children: [
                    Expanded(
                      child: Row(
                        children: [
                          _CallButton(icon: Icons.mic_rounded, label: 'Микрофон', hint: 'вкл / выкл', onTap: () => _do(context, 'mic')),
                          const SizedBox(width: 12),
                          _CallButton(
                            icon: camera ? Icons.videocam_rounded : Icons.videocam_off_rounded,
                            label: camera ? 'Камера включена' : 'Камера выключена',
                            hint: camera ? 'тап — выключить' : 'тап — включить',
                            color: camera ? C.ok : C.danger,
                            onTap: () => _do(context, 'camera'),
                          ),
                        ],
                      ),
                    ),
                    const SizedBox(height: 12),
                    Expanded(
                      child: Row(
                        children: [
                          _CallButton(icon: Icons.back_hand_rounded, label: 'Поднять руку', onTap: () => _do(context, 'hand')),
                          const SizedBox(width: 12),
                          _CallButton(icon: Icons.open_in_new_rounded, label: 'Открыть Meet', onTap: () => _do(context, 'show')),
                        ],
                      ),
                    ),
                  ],
                ),
              ),
            ),
          ],
        );
      },
    );
  }
}

class _CallButton extends StatelessWidget {
  const _CallButton({required this.icon, required this.label, required this.onTap, this.hint, this.color});
  final IconData icon;
  final String label;
  final String? hint;
  final Color? color;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final c = color ?? C.accent;
    return Expanded(
      child: Material(
        color: C.tile,
        borderRadius: BorderRadius.circular(20),
        child: InkWell(
          borderRadius: BorderRadius.circular(20),
          onTap: onTap,
          child: LayoutBuilder(
            builder: (context, box) {
              final s = box.biggest.shortestSide;
              return Column(
                mainAxisAlignment: MainAxisAlignment.center,
                children: [
                  Icon(icon, size: (s * .38).clamp(24.0, 56.0), color: c),
                  SizedBox(height: s * .05),
                  Text(
                    label,
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: const TextStyle(fontWeight: FontWeight.w600),
                  ),
                  if (hint != null) Text(hint!, style: const TextStyle(color: C.muted, fontSize: 12)),
                ],
              );
            },
          ),
        ),
      ),
    );
  }
}
