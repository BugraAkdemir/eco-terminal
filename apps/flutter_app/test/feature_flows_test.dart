import 'package:eco_terminal/core/models/models.dart';
import 'package:eco_terminal/core/extensions/extension_registry.dart';
import 'package:eco_terminal/features/intelligence/presentation/intelligence_tabs.dart';
import 'package:fl_chart/fl_chart.dart';
import 'package:eco_terminal/core/router/app_router.dart';
import 'package:eco_terminal/features/maps/domain/map_state.dart';
import 'package:eco_terminal/features/observations/data/draft_queue.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'helpers/fakes.dart';

void main() {
  testWidgets(
    'six-step wizard saves a resumable draft and opens the local queue',
    (tester) async {
      final adapter = emptyApi()..on('GET /auth/me', {'data': userJson()});
      final c = await pumpApp(
        tester,
        size: const Size(400, 900),
        adapter: adapter,
        token: 'tok',
      );
      c.read(routerProvider).go('/observations/new');
      for (var i = 0; i < 4; i++) {
        await tester.pump(const Duration(milliseconds: 100));
      }
      expect(find.text('1 / 6'), findsOneWidget);
      for (var i = 0; i < 2; i++) {
        await tester.tap(find.byKey(const ValueKey('wizard-next')));
        await tester.pump(const Duration(milliseconds: 100));
      }
      await tester.enterText(
        find.widgetWithText(TextField, 'Enlem'),
        '41.3887',
      );
      await tester.enterText(
        find.widgetWithText(TextField, 'Boylam'),
        '33.7827',
      );
      await tester.tap(find.byKey(const ValueKey('wizard-next')));
      await tester.pump();
      await tester.enterText(
        find.widgetWithText(TextField, 'Kısa başlık'),
        'Sunum saha kaydı',
      );
      await tester.tap(find.byKey(const ValueKey('wizard-next')));
      await tester.pump();
      await tester.tap(find.byKey(const ValueKey('wizard-next')));
      await tester.pump();
      expect(find.text('6 / 6'), findsOneWidget);
      // Save and leave through the explicit draft action; no network submission.
      await tester.tap(find.byTooltip('Taslağı sakla ve çık'));
      for (var i = 0; i < 5; i++) {
        await tester.pump(const Duration(milliseconds: 100));
      }
      expect(find.text('Saha kayıtlarım'), findsOneWidget);
      final drafts = c.read(draftQueueProvider('u1')).requireValue;
      expect(drafts.single.body['title'], 'Sunum saha kaydı');
      expect(drafts.single.status, DraftStatus.draft);
      expect(
        adapter.requests.where(
          (r) => r.method == 'POST' && r.path == '/observations',
        ),
        isEmpty,
      );
      expect(tester.takeException(), isNull);
      await tester.pumpWidget(const SizedBox.shrink());
      c.dispose();
      await tester.pump();
    },
  );
  testWidgets(
    'assistant sends selected region and shows a cited fallback answer',
    (tester) async {
      final adapter = emptyApi()
        ..on('GET /auth/me', {'data': userJson()})
        ..on('POST /ai/chat', {
          'data': {
            'answer': 'Kayıtlarda 7 gözlem var. [1]',
            'mode': 'retrieval_only',
            'facts': [],
            'sources': [
              {
                'id': '1',
                'type': 'sql_fact',
                'title': 'Gözlem sayısı',
                'snippet': 'SQL sonucu: 7',
              },
            ],
            'limitations': [],
            'period': {},
          },
        });
      final c = await pumpApp(
        tester,
        size: const Size(400, 900),
        adapter: adapter,
        token: 'tok',
      );
      c
          .read(selectedRegionProvider.notifier)
          .select(
            Region.fromJson({'id': 'r1', 'name': 'Merkez', 'kind': 'district'}),
          );
      c.read(routerProvider).go('/ai');
      for (var i = 0; i < 4; i++) {
        await tester.pump(const Duration(milliseconds: 100));
      }
      await tester.enterText(
        find.byKey(const ValueKey('assistant-question')),
        'Kaç gözlem var?',
      );
      await tester.tap(find.byKey(const ValueKey('assistant-send')));
      for (var i = 0; i < 4; i++) {
        await tester.pump(const Duration(milliseconds: 100));
      }
      expect(find.text('Kayıtlarda 7 gözlem var. [1]'), findsOneWidget);
      final sent = adapter.requests.singleWhere((r) => r.path == '/ai/chat');
      expect((sent.data as Map)['region_id'], 'r1');
      expect(find.textContaining('Kayıtlı verilerden özet'), findsOneWidget);
      expect(tester.takeException(), isNull);
      await tester.pumpWidget(const SizedBox.shrink());
      c.dispose();
      await tester.pump();
    },
  );
  testWidgets(
    'expert can review a pending record and explicitly confirm verification',
    (tester) async {
      final observation = {
        'id': 'review-one',
        'title': 'İncelenecek kayıt',
        'category': 'animal',
        'record_type': 'observation',
        'status': 'submitted',
        'lat': 41.3,
        'lon': 33.7,
        'observed_at': '2026-10-19T10:00:00Z',
        'media': [],
        'ai_analysis': [],
        'history': [],
        'reviews': [],
      };
      final adapter = emptyApi()
        ..on('GET /auth/me', {'data': userJson(role: 'admin')})
        ..on('GET /reviews/queue', {
          'data': [observation],
          'meta': {'total': 1},
        })
        ..on('POST /reviews/review-one/decision', {
          'data': {...observation, 'status': 'verified'},
        });
      final c = await pumpApp(
        tester,
        size: const Size(1400, 1000),
        adapter: adapter,
        token: 'tok',
      );
      c.read(routerProvider).go('/expert');
      for (var i = 0; i < 5; i++) {
        await tester.pump(const Duration(milliseconds: 100));
      }
      await tester.tap(find.widgetWithText(OutlinedButton, 'Onayla'));
      await tester.pump();
      expect(find.text('Uzman kararını kaydet?'), findsOneWidget);
      await tester.tap(find.widgetWithText(FilledButton, 'Onayla'));
      for (var i = 0; i < 4; i++) {
        await tester.pump(const Duration(milliseconds: 100));
      }
      final sent = adapter.requests.singleWhere(
        (r) => r.path == '/reviews/review-one/decision',
      );
      expect((sent.data as Map)['decision'], 'verify');
      expect(tester.takeException(), isNull);
      await tester.pumpWidget(const SizedBox.shrink());
      c.dispose();
      await tester.pump();
    },
  );
  testWidgets(
    'scenario sends real parameters and displays a chart with missing-data gaps',
    (tester) async {
      final adapter = emptyApi()
        ..on('GET /auth/me', {'data': userJson(role: 'admin')})
        ..on('GET /simulations', {'data': []})
        ..on('POST /simulations/run', {
          'data': {
            'id': 'sim-one',
            'name': 'Test scenario',
            'status': 'ok',
            'indicators': [
              {
                'key': 'annual_precip',
                'label': 'Annual rain',
                'unit': 'mm',
                'reference': 600,
                'scenario': 480,
                'change': -120,
              },
            ],
            'series': [
              {
                'key': 'precipitation',
                'label': 'Rainfall',
                'unit': 'mm',
                'points': [
                  {'month': '2020-01', 'reference': 50, 'scenario': 40},
                  {'month': '2020-02', 'reference': null, 'scenario': null},
                  {'month': '2020-03', 'reference': 30, 'scenario': 24},
                ],
              },
            ],
            'data_sources': [],
            'limitations': [],
            'assumptions': [],
          },
        });
      final registry = ExtensionRegistry()
        ..registerRoute('/scenario-test', (c, s) => const SimulationsTab());
      final c = await pumpApp(
        tester,
        size: const Size(400, 1000),
        adapter: adapter,
        token: 'tok',
        registry: registry,
      );
      c
          .read(selectedRegionProvider.notifier)
          .select(
            Region.fromJson({
              'id': 'r1',
              'name': 'Kastamonu',
              'kind': 'province',
            }),
          );
      c.read(routerProvider).go('/scenario-test');
      for (var i = 0; i < 4; i++) {
        await tester.pump(const Duration(milliseconds: 100));
      }
      await tester.scrollUntilVisible(
        find.byKey(const ValueKey('simulation-run')),
        300,
        scrollable: find
            .descendant(
              of: find.byType(SimulationsTab),
              matching: find.byType(Scrollable),
            )
            .first,
      );
      await tester.tap(find.byKey(const ValueKey('simulation-run')));
      for (var i = 0; i < 5; i++) {
        await tester.pump(const Duration(milliseconds: 100));
      }
      await tester.scrollUntilVisible(
        find.byType(LineChart),
        300,
        scrollable: find
            .descendant(
              of: find.byType(SimulationsTab),
              matching: find.byType(Scrollable),
            )
            .first,
      );
      final request = adapter.requests.singleWhere(
        (r) => r.path == '/simulations/run',
      );
      expect((request.data as Map)['region_id'], 'r1');
      expect((request.data as Map)['precip_change_pct'], -20);
      expect((request.data as Map)['temp_change_c'], 2);
      expect((request.data as Map)['scale'], 3);
      final chart = tester.widget<LineChart>(find.byType(LineChart));
      expect(chart.data.lineBarsData.first.spots[1], FlSpot.nullSpot);
      expect(tester.takeException(), isNull);
      await tester.pumpWidget(const SizedBox.shrink());
      c.dispose();
      await tester.pump();
    },
  );
}
