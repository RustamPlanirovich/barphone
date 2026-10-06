// Wake-on-LAN magic packets. Pure Dart.
import 'dart:io';
import 'dart:typed_data';

List<int>? parseMac(String mac) {
  final parts = mac.split(RegExp(r'[:-]'));
  if (parts.length != 6) return null;
  final out = <int>[];
  for (final p in parts) {
    final v = int.tryParse(p, radix: 16);
    if (v == null || v < 0 || v > 255) return null;
    out.add(v);
  }
  return out;
}

Uint8List magicPacket(List<int> mac) {
  final b = BytesBuilder();
  b.add(List.filled(6, 0xff));
  for (var i = 0; i < 16; i++) {
    b.add(mac);
  }
  return b.toBytes();
}

/// Broadcasts magic packets for [macs]. [lastHost] (the PC's last known IPv4) adds a
/// directed /24 broadcast, which some routers forward where 255.255.255.255 is dropped.
Future<int> wake(List<String> macs, {String? lastHost}) async {
  final targets = <InternetAddress>[InternetAddress('255.255.255.255')];
  final ip = lastHost == null ? null : InternetAddress.tryParse(lastHost);
  if (ip != null && ip.type == InternetAddressType.IPv4) {
    final r = ip.rawAddress;
    targets.add(InternetAddress('${r[0]}.${r[1]}.${r[2]}.255'));
  }
  final sock = await RawDatagramSocket.bind(InternetAddress.anyIPv4, 0);
  sock.broadcastEnabled = true;
  var sent = 0;
  try {
    for (final m in macs) {
      final mac = parseMac(m);
      if (mac == null) continue;
      final packet = magicPacket(mac);
      for (final t in targets) {
        for (final port in const [9, 7]) {
          if (sock.send(packet, t, port) > 0) sent++;
        }
      }
    }
  } finally {
    sock.close();
  }
  return sent;
}
