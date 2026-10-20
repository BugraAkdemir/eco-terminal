import 'package:eco_terminal/core/extensions/extension_registry.dart';
import 'package:eco_terminal/core/l10n/strings.dart';
import 'package:eco_terminal/core/models/models.dart';
import 'package:eco_terminal/core/session/session_controller.dart';
import 'package:eco_terminal/features/maps/domain/map_state.dart';
import 'package:eco_terminal/shared_widgets/shared_widgets.dart';
import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_test/flutter_test.dart';

import 'helpers/fakes.dart';

Map<String, dynamic> region() => {
      'id': 'r1',
      'kind': 'district',
      'name': 'Merkez',
      'bbox': [33.0, 41.0, 34.0, 42.0],
    };

void main() {
  group('responsive shell', () {
    testWidgets('desktop (1400): resizable panel with 5 tabs next to the map',
        (tester) async {
      await pumpApp(tester, size: const Size(1400, 900));
      expect(find.byKey(const ValueKey('desktop-panel')), findsOneWidget);
      expect(find.byKey(const ValueKey('panel-resize-handle')), findsOneWidget);
      expect(find.byType(NavigationBar), findsNothing);
      for (final id in PanelTabIds.all) {
        expect(find.byKey(ValueKey('panel-tab-$id')), findsOneWidget, reason: id);
      }
      expect(tester.takeException(), isNull);

      // Extension tab shows the honest placeholder, not fake data.
      await tester.tap(find.byKey(const ValueKey('panel-tab-ai_assistant')));
      await tester.pump();
      expect(find.text('Bu modül henüz yapılandırılmadı'), findsOneWidget);

      // Panel resizes by dragging the handle and collapses to a rail.
      final before = tester.getSize(find.byKey(const ValueKey('desktop-panel'))).width;
      await tester.drag(
        find.byKey(const ValueKey('panel-resize-handle')),
        const Offset(120, 0),
      );
      await tester.pump();
      final after = tester.getSize(find.byKey(const ValueKey('desktop-panel'))).width;
      expect(after, greaterThan(before));
      await tester.tap(find.byTooltip('Paneli daralt'));
      await tester.pump();
      expect(find.byKey(const ValueKey('desktop-panel-rail')), findsOneWidget);
    });

    testWidgets('tablet (900): overlay panel, no bottom nav', (tester) async {
      await pumpApp(tester, size: const Size(900, 800));
      expect(find.byKey(const ValueKey('tablet-panel')), findsOneWidget);
      expect(find.byKey(const ValueKey('desktop-panel')), findsNothing);
      expect(find.byType(NavigationBar), findsNothing);
      expect(tester.takeException(), isNull);
    });

    testWidgets('mobile (400): bottom nav Harita / Gözlem Ekle / AI / Profil',
        (tester) async {
      await pumpApp(tester, size: const Size(400, 800));
      expect(find.byType(NavigationBar), findsOneWidget);
      for (final label in ['Harita', 'Gözlem Ekle', 'AI', 'Profil']) {
        expect(
          find.descendant(of: find.byType(NavigationBar), matching: find.text(label)),
          findsOneWidget,
          reason: label,
        );
      }
      expect(find.byKey(const ValueKey('desktop-panel')), findsNothing);
      expect(find.byKey(const ValueKey('mobile-open-panel')), findsOneWidget);
      expect(tester.takeException(), isNull);
    });
  });

  testWidgets('selecting a region updates the active chip and the Information tab',
      (tester) async {
    final adapter = emptyApi()
      ..on('GET /regions', {
        'data': [region()],
      })
      ..on('GET /regions/r1/analytics', {
        'data': {
          'region_id': 'r1',
          'totals': {'observations': 7, 'issues': 1, 'verified': 3, 'pending_review': 2},
          'by_category': [
            {'category': 'animal', 'count': 5},
            {'category': 'plant', 'count': 2},
          ],
          'by_month': [
            {'month': '2026-04', 'count': 7, 'distinct_observers': 1},
          ],
          'data_quality': {
            'distinct_observers': 1,
            'months_with_data': 1,
            'months_in_period': 12,
            'warnings': ['Tek gözlemci: sonuçlar yanıltıcı olabilir.'],
          },
        },
      })
      ..on('GET /regions/r1/observations', {
        'data': [
          {
            'id': 'o1',
            'title': 'Karaca',
            'category': 'animal',
            'status': 'verified',
            'lat': 41.5,
            'lon': 33.5,
            'observed_at': '2026-04-01T10:00:00Z',
          },
        ],
        'meta': {'page': 1, 'page_size': 5, 'total': 1},
      });
    final container = await pumpApp(tester, size: const Size(1400, 900), adapter: adapter);

    expect(find.text('Bölge seçilmedi'), findsWidgets);
    container
        .read(selectedRegionProvider.notifier)
        .select(Region.fromJson(region()));
    for (var i = 0; i < 5; i++) {
      await tester.pump(const Duration(milliseconds: 200));
    }

    expect(find.textContaining('Aktif bölge: Merkez'), findsWidgets);
    expect(find.text('7'), findsOneWidget); // observations metric
    expect(find.byKey(const ValueKey('data-quality-warnings')), findsOneWidget);
    await tester.scrollUntilVisible(
      find.text('Karaca'),
      300,
      scrollable: find.descendant(
        of: find.byKey(const ValueKey('tab-information')),
        matching: find.byType(Scrollable),
      ),
    );
    expect(find.text('Karaca'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });

  testWidgets('auth: login validation and 401 redirect', (tester) async {
    await pumpApp(tester, size: const Size(1400, 900));
    await tester.tap(find.text('Giriş yap').first);
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 100));
    expect(find.byKey(const ValueKey('login-submit')), findsOneWidget);

    await tester.enterText(find.byKey(const ValueKey('login-email')), 'bozuk');
    await tester.tap(find.byKey(const ValueKey('login-submit')));
    await tester.pump();
    expect(find.text('Geçerli bir e-posta adresi girin.'), findsOneWidget);
  });

  testWidgets('session: 401 clears the session and lands on login', (tester) async {
    final adapter = emptyApi()
      ..on('GET /auth/me', {'data': userJson()})
      ..on('GET /notifications', {
        'error': {'code': 'unauthorized', 'message': 'expired'},
      }, status: 401);
    final container =
        await pumpApp(tester, size: const Size(1400, 900), adapter: adapter, token: 'tok');
    await tester.pump(const Duration(milliseconds: 300));
    expect(container.read(sessionProvider).isAuthenticated, isFalse);
    expect(container.read(sessionProvider).token, isNull);
    expect(find.byKey(const ValueKey('login-submit')), findsOneWidget);
  });

  group('accessibility', () {
    testWidgets('status badges: 8 statuses, label + hint semantics, tap explains',
        (tester) async {
      await tester.pumpWidget(
        MaterialApp(
          locale: const Locale('tr'),
          supportedLocales: AppStrings.supportedLocales,
          localizationsDelegates: GlobalMaterialLocalizations.delegates,
          home: Scaffold(
            body: Column(
              children: [
                for (final s in ObservationStatus.lifecycle) StatusBadge(status: s),
              ],
            ),
          ),
        ),
      );
      final handle = tester.ensureSemantics();
      expect(
        find.bySemanticsLabel('Durum: Uzman onayladı'),
        findsOneWidget,
      );
      expect(find.bySemanticsLabel('Durum: Yapay zekâ ön inceleme yaptı'), findsOneWidget);
      // Icon + text, not colour alone.
      expect(find.byIcon(Icons.verified), findsOneWidget);
      await tester.tap(find.text('Uzman inceliyor'));
      await tester.pump();
      expect(find.text('Bir uzman kaydınızı inceliyor.'), findsOneWidget);
      handle.dispose();
    });

    testWidgets('text scale 200%: status tags and mobile shell do not overflow',
        (tester) async {
      tester.view.physicalSize = const Size(360, 800);
      tester.view.devicePixelRatio = 1;
      tester.platformDispatcher.textScaleFactorTestValue = 2.0;
      addTearDown(() {
        tester.view.resetPhysicalSize();
        tester.view.resetDevicePixelRatio();
        tester.platformDispatcher.clearTextScaleFactorTestValue();
      });
      await tester.pumpWidget(
        MaterialApp(
          locale: const Locale('tr'),
          supportedLocales: AppStrings.supportedLocales,
          localizationsDelegates: GlobalMaterialLocalizations.delegates,
          home: Scaffold(
            body: SingleChildScrollView(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  for (final s in ObservationStatus.lifecycle) StatusBadge(status: s),
                  const EmergencyNotice(),
                  const MetricCard(label: 'İnceleme bekleyen', value: '1.234'),
                ],
              ),
            ),
          ),
        ),
      );
      expect(tester.takeException(), isNull);
    });

    testWidgets('text scale 200%: full app at phone width renders without overflow',
        (tester) async {
      await pumpApp(tester, size: const Size(360, 800), textScale: 2.0);
      expect(tester.takeException(), isNull);
      expect(find.byType(NavigationBar), findsOneWidget);
    });
  });
}
