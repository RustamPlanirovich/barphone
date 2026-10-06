import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../protocol/link.dart';
import '../protocol/models.dart';
import 'theme.dart';

/// Controls of a Google Meet call on the PC: a narrow strip (one tile wide) next to the
/// deck when only one computer is connected. Whether the microphone is on cannot be known
/// (Meet keeps it open while muted), so that button is a plain toggle; the camera is.
class CallPanel extends StatelessWidget {
  const CallPanel({super.key, required this.link});
  final MachineLink link;

  Future<void> _do(BuildContext context, String action) async {
    if (action == 'leave' && !await _confirmLeave(context)) return;
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

  Future<bool> _confirmLeave(BuildContext context) async {
    HapticFeedback.mediumImpact();
    final ok = await showDialog<bool>(
      context: context,
      builder: (d) => AlertDialog(
        title: const Text('Выйти из звонка?'),
        content: Text('Вкладка Meet на «${link.machine.name}» закроется.'),
        actions: [
          TextButton(onPressed: () => Navigator.pop(d, false), child: const Text('Остаться')),
          FilledButton(
            style: FilledButton.styleFrom(backgroundColor: C.danger, foregroundColor: Colors.white),
            onPressed: () => Navigator.pop(d, true),
            child: const Text('Выйти'),
          ),
        ],
      ),
    );
    return ok == true;
  }

  @override
  Widget build(BuildContext context) {
    return ValueListenableBuilder<CallInfo?>(
      valueListenable: link.call,
      builder: (context, call, _) {
        final camera = call?.camera ?? false;
        final buttons = [
          _CallButton(icon: Icons.mic_rounded, label: 'Микрофон', onTap: () => _do(context, 'mic')),
          _CallButton(
            icon: camera ? Icons.videocam_rounded : Icons.videocam_off_rounded,
            label: camera ? 'Камера вкл' : 'Камера выкл',
            color: camera ? C.ok : C.muted,
            onTap: () => _do(context, 'camera'),
          ),
          _CallButton(icon: Icons.back_hand_rounded, label: 'Рука', onTap: () => _do(context, 'hand')),
          _CallButton(icon: Icons.open_in_new_rounded, label: 'Открыть', onTap: () => _do(context, 'show')),
          _CallButton(icon: Icons.call_end_rounded, label: 'Выйти', color: C.danger, onTap: () => _do(context, 'leave')),
        ];
        return Padding(
          padding: const EdgeInsets.fromLTRB(6, 8, 10, 8),
          child: Column(
            children: [
              const Padding(
                padding: EdgeInsets.only(bottom: 6),
                child: Row(
                  mainAxisAlignment: MainAxisAlignment.center,
                  children: [
                    Icon(Icons.video_call_rounded, color: C.ok, size: 18),
                    SizedBox(width: 4),
                    Flexible(
                      child: Text(
                        'Meet',
                        maxLines: 1,
                        overflow: TextOverflow.ellipsis,
                        style: TextStyle(fontSize: 13, fontWeight: FontWeight.w600, color: C.ok),
                      ),
                    ),
                  ],
                ),
              ),
              for (final (i, b) in buttons.indexed) ...[if (i > 0) const SizedBox(height: 6), Expanded(child: b)],
            ],
          ),
        );
      },
    );
  }
}

class _CallButton extends StatelessWidget {
  const _CallButton({required this.icon, required this.label, required this.onTap, this.color});
  final IconData icon;
  final String label;
  final Color? color;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final c = color ?? C.accent;
    return Semantics(
      button: true,
      label: label,
      child: Material(
        color: C.tile,
        borderRadius: BorderRadius.circular(14),
        child: InkWell(
          borderRadius: BorderRadius.circular(14),
          onTap: onTap,
          child: LayoutBuilder(
            builder: (context, box) {
              final h = box.maxHeight;
              final withLabel = h >= 44; // too short: the icon alone
              return Center(
                child: Column(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    Icon(icon, size: (h * (withLabel ? .42 : .6)).clamp(16.0, 30.0), color: c),
                    if (withLabel)
                      Text(
                        label,
                        maxLines: 1,
                        overflow: TextOverflow.ellipsis,
                        style: const TextStyle(fontSize: 11, color: C.muted, height: 1.2),
                      ),
                  ],
                ),
              );
            },
          ),
        ),
      ),
    );
  }
}
