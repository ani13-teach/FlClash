import 'package:fl_clash/features/connection/connection_history.dart';
import 'package:fl_clash/models/models.dart';
import 'package:flutter_test/flutter_test.dart';

TrackerInfo _tracker(String id, DateTime start, {int download = 0}) {
  return TrackerInfo(
    id: id,
    start: start,
    download: download,
    metadata: const Metadata(network: 'tcp', host: 'example.com'),
    chains: const ['DIRECT'],
    rule: 'MATCH',
    rulePayload: '',
  );
}

void main() {
  late ConnectionHistory history;
  final now = DateTime.utc(2026, 10, 20, 12);

  setUp(() {
    history = ConnectionHistory();
  });

  test('sorts by newest start regardless of traffic or snapshot order', () {
    final old = _tracker('old', now, download: 10000);
    final recent = _tracker('recent', now.add(const Duration(seconds: 1)));
    final newest = _tracker('newest', now.add(const Duration(seconds: 2)));

    expect(history.update([old, newest, recent], now), [newest, recent, old]);
    expect(history.update([recent, old, newest], now), [newest, recent, old]);
  });

  test('breaks equal start times by id', () {
    final a = _tracker('a', now);
    final b = _tracker('b', now);

    expect(history.update([b, a], now), [a, b]);
    expect(history.update([a, b], now), [a, b]);
  });

  test('refreshes traffic without duplicating or reordering rows', () {
    final old = _tracker('old', now);
    final recent = _tracker('recent', now.add(const Duration(seconds: 1)));
    history.update([old, recent], now);
    final updated = old.copyWith(upload: 100000, download: 200000);

    expect(
      history.update([recent, updated], now.add(const Duration(seconds: 1))),
      [recent, updated],
    );
    expect(history.isActive('old'), isTrue);
  });

  test('retains ended connections through the 30 minute boundary', () {
    final connection = _tracker('a', now);
    history.update([connection], now);

    expect(history.update([], now.add(const Duration(minutes: 29))), [
      connection,
    ]);
    expect(history.isActive('a'), isFalse);
    expect(history.update([], now.add(ConnectionHistory.retention)), [
      connection,
    ]);
    expect(
      history.update(
        [],
        now.add(ConnectionHistory.retention + const Duration(microseconds: 1)),
      ),
      isEmpty,
    );
  });

  test('retention starts from the last active snapshot', () {
    final connection = _tracker('a', now.subtract(const Duration(hours: 2)));
    history.update([connection], now);
    final lastSeen = now.add(const Duration(hours: 1));
    final updated = connection.copyWith(download: 500);
    history.update([updated], lastSeen);

    expect(history.update([], lastSeen.add(const Duration(minutes: 30))), [
      updated,
    ]);
    expect(
      history.update([], lastSeen.add(const Duration(minutes: 31))),
      isEmpty,
    );
  });

  test('never expires a connection that is still active', () {
    final connection = _tracker('a', now.subtract(const Duration(hours: 2)));
    history.update([connection], now);

    expect(history.update([connection], now.add(const Duration(days: 1))), [
      connection,
    ]);
    expect(history.isActive('a'), isTrue);
  });

  test('merges recent history and new connections in time order', () {
    final ended = _tracker('ended', now);
    final active = _tracker('active', now.add(const Duration(minutes: 1)));
    final newest = _tracker('newest', now.add(const Duration(minutes: 2)));
    history.update([active, ended], now);

    expect(
      history.update([newest, active], now.add(const Duration(minutes: 2))),
      [newest, active, ended],
    );
    expect(history.isActive('ended'), isFalse);
    expect(history.isActive('active'), isTrue);
    expect(history.isActive('newest'), isTrue);
  });

  test('a reappearing id updates its entry and retention window', () {
    final connection = _tracker('a', now);
    history.update([connection], now);
    history.update([], now.add(const Duration(minutes: 1)));
    final updated = connection.copyWith(download: 100);
    history.update([updated], now.add(const Duration(minutes: 29)));

    expect(history.isActive('a'), isTrue);
    expect(history.update([], now.add(const Duration(minutes: 59))), [updated]);
    expect(history.update([], now.add(const Duration(minutes: 60))), isEmpty);
  });

  test('only expires old history when another connection remains active', () {
    final old = _tracker('old', now);
    final active = _tracker('active', now);
    history.update([old, active], now);

    expect(history.update([active], now.add(const Duration(minutes: 31))), [
      active,
    ]);
    expect(history.isActive('old'), isFalse);
    expect(history.isActive('active'), isTrue);
  });
}
