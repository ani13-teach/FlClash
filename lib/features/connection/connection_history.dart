import 'package:fl_clash/models/models.dart';

class ConnectionHistory {
  static const retention = Duration(minutes: 30);

  final _entries = <String, ({TrackerInfo info, DateTime lastSeen})>{};
  Set<String> _activeIds = {};

  bool isActive(String id) => _activeIds.contains(id);

  List<TrackerInfo> update(List<TrackerInfo> connections, DateTime now) {
    _activeIds = {for (final connection in connections) connection.id};
    for (final connection in connections) {
      _entries[connection.id] = (info: connection, lastSeen: now);
    }
    _entries.removeWhere(
      (id, entry) =>
          !isActive(id) && now.difference(entry.lastSeen) > retention,
    );
    return _entries.values.map((entry) => entry.info).toList()..sort((a, b) {
      final start = b.start.compareTo(a.start);
      return start != 0 ? start : a.id.compareTo(b.id);
    });
  }
}
