import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../core/router/route_access.dart';
import '../../../core/theme/tokens.dart';
import 'observation_detail.dart';

/// Default `/observations/:id` page (an extension may replace it).
class ObservationPage extends ConsumerWidget {
  const ObservationPage({super.key, required this.id});
  final String id;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    return Center(
      child: ConstrainedBox(
        constraints: const BoxConstraints(maxWidth: 720),
        child: Padding(
          padding: const EdgeInsets.all(Dimens.lg),
          child: ObservationDetailCard(
            observationId: id,
            onClose: () {
              if (context.canPop()) {
                context.pop();
              } else {
                context.go(AppPaths.home);
              }
            },
          ),
        ),
      ),
    );
  }
}
