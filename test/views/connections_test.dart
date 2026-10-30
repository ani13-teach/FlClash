import 'package:fl_clash/common/common.dart';
import 'package:fl_clash/core/controller.dart';
import 'package:fl_clash/core/interface.dart';
import 'package:fl_clash/features/features.dart';
import 'package:fl_clash/models/models.dart';
import 'package:fl_clash/providers/core.dart';
import 'package:fl_clash/providers/database.dart';
import 'package:fl_clash/state.dart';
import 'package:fl_clash/views/connection/connections.dart';
import 'package:fl_clash/widgets/null_status.dart';
import 'package:material_ui/material_ui.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mocktail/mocktail.dart';

import '../helpers/test_app.dart';
import '../helpers/test_profiles.dart';

class MockCoreHandlerInterface extends Mock implements CoreHandlerInterface {}

TrackerInfo _tracker({
  required String id,
  String host = 'example.com',
  String process = 'curl',
  List<String> chains = const ['Proxy'],
  int upload = 0,
  int download = 0,
  DateTime? start,
}) {
  return TrackerInfo(
    id: id,
    upload: upload,
    download: download,
    start: start ?? DateTime.utc(2026),
    metadata: Metadata(
      network: 'tcp',
      host: host,
      destinationIP: '1.1.1.1',
      destinationPort: '443',
      process: process,
    ),
    chains: chains,
    rule: 'DOMAIN',
    rulePayload: host,
  );
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  late MockCoreHandlerInterface core;
  late ProviderContainer container;

  setUp(() {
    core = MockCoreHandlerInterface();
    container = ProviderContainer(
      overrides: [
        profilesProvider.overrideWith(TestProfiles.new),
        coreHandlerProvider.overrideWithValue(CoreController.scoped(core)),
      ],
    );
    globalState.container = container;
  });

  tearDown(() => container.dispose());

  Future<void> pumpConnections(WidgetTester tester) async {
    tester.view.physicalSize = const Size(1400, 1000);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    await tester.pumpWidget(
      UncontrolledProviderScope(
        container: container,
        child: const TestApp(child: ConnectionsView()),
      ),
    );
    await tester.pump();
    await tester.pump();
  }

  Future<void> teardownView(WidgetTester tester) async {
    await tester.pumpWidget(const SizedBox.shrink());
    await tester.pump(const Duration(seconds: 2));
  }

  testWidgets('shows the empty state when core reports no connections', (
    tester,
  ) async {
    when(core.getConnections).thenAnswer((_) async => const <TrackerInfo>[]);

    await pumpConnections(tester);

    expect(find.byType(NullStatus), findsOneWidget);
    expect(tester.takeException(), null);

    await teardownView(tester);
  });

  testWidgets('renders one row per reported connection', (tester) async {
    when(core.getConnections).thenAnswer(
      (_) async => [
        _tracker(id: 'a', host: 'alpha.test'),
        _tracker(id: 'b', host: 'beta.test'),
      ],
    );

    await pumpConnections(tester);
    await tester.pump(commonDuration);
    await tester.pump(commonDuration);

    expect(find.byType(NullStatus), findsNothing);
    expect(find.textContaining('alpha.test'), findsWidgets);
    expect(find.textContaining('beta.test'), findsWidgets);
    expect(tester.takeException(), null);

    await teardownView(tester);
  });

  testWidgets('orders rows by newest start regardless of traffic', (
    tester,
  ) async {
    when(core.getConnections).thenAnswer(
      (_) async => [
        _tracker(id: 'a', host: 'alpha.test', download: 100),
        _tracker(id: 'b', host: 'beta.test', upload: 300, download: 300),
        _tracker(
          id: 'c',
          host: 'gamma.test',
          download: 100,
          start: DateTime.utc(2026, 2),
        ),
      ],
    );

    await pumpConnections(tester);

    double topOf(String host) =>
        tester.getTopLeft(find.textContaining(host).first).dy;
    expect(topOf('gamma.test'), lessThan(topOf('alpha.test')));
    expect(topOf('alpha.test'), lessThan(topOf('beta.test')));
    expect(tester.takeException(), null);

    await teardownView(tester);
  });

  IconButton blockButton(WidgetTester tester, String id) {
    final row = tester.widget<TrackerInfoItem>(find.byKey(Key(id)));
    return row.trailing! as IconButton;
  }

  testWidgets('keeps ended rows and disables their block buttons', (
    tester,
  ) async {
    var connections = [
      _tracker(id: 'a', host: 'alpha.test'),
      _tracker(id: 'b', host: 'beta.test'),
    ];
    when(core.getConnections).thenAnswer((_) async => connections);
    await pumpConnections(tester);

    expect(blockButton(tester, 'a').onPressed, isNotNull);
    connections = [connections.last];
    await tester.pump(const Duration(seconds: 1));
    await tester.pump();

    expect(find.textContaining('alpha.test'), findsWidgets);
    expect(blockButton(tester, 'a').onPressed, isNull);
    expect(blockButton(tester, 'b').onPressed, isNotNull);
    expect(tester.takeException(), null);

    await teardownView(tester);
  });

  testWidgets('closing one connection preserves its displayed record', (
    tester,
  ) async {
    var connections = [_tracker(id: 'a', host: 'alpha.test')];
    when(core.getConnections).thenAnswer((_) async => connections);
    when(() => core.closeConnection('a')).thenAnswer((_) async {
      connections = [];
      return true;
    });
    await pumpConnections(tester);

    await tester.tap(find.byIcon(Icons.block));
    await tester.pump();
    await tester.pump(commonDuration);

    verify(() => core.closeConnection('a')).called(1);
    expect(find.textContaining('alpha.test'), findsWidgets);
    expect(blockButton(tester, 'a').onPressed, isNull);
    expect(tester.takeException(), null);

    await teardownView(tester);
  });

  testWidgets('closing all connections preserves their displayed records', (
    tester,
  ) async {
    var connections = [_tracker(id: 'a', host: 'alpha.test')];
    when(core.getConnections).thenAnswer((_) async => connections);
    when(core.closeConnections).thenAnswer((_) async {
      connections = [];
      return true;
    });
    await pumpConnections(tester);

    await tester.tap(find.byIcon(Icons.delete_sweep_outlined));
    await tester.pump();
    await tester.pump(commonDuration);

    verify(core.closeConnections).called(1);
    expect(find.textContaining('alpha.test'), findsWidgets);
    expect(blockButton(tester, 'a').onPressed, isNull);
    expect(tester.takeException(), null);

    await teardownView(tester);
  });

  testWidgets('keeps existing records when a later poll fails', (tester) async {
    when(
      core.getConnections,
    ).thenAnswer((_) async => [_tracker(id: 'a', host: 'alpha.test')]);
    await pumpConnections(tester);
    when(core.getConnections).thenThrow(StateError('core down'));

    await tester.pump(const Duration(seconds: 1));
    await tester.pump();

    expect(find.textContaining('alpha.test'), findsWidgets);
    expect(tester.takeException(), null);

    await teardownView(tester);
  });

  testWidgets('keeps the empty state when core throws', (tester) async {
    when(core.getConnections).thenThrow(StateError('core down'));

    await pumpConnections(tester);

    expect(find.byType(NullStatus), findsOneWidget);
    expect(tester.takeException(), null);

    await teardownView(tester);
  });

  testWidgets('stops polling once the view is disposed', (tester) async {
    when(core.getConnections).thenAnswer((_) async => const <TrackerInfo>[]);

    await pumpConnections(tester);
    await teardownView(tester);
    clearInteractions(core);

    await tester.pump(const Duration(seconds: 3));

    verifyNever(core.getConnections);
  });
}
