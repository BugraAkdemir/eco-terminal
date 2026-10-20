import 'dart:io';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'lib/app.dart';
import 'lib/core/api/providers.dart';
import 'lib/core/l10n/format.dart';
import 'lib/core/router/app_router.dart';
import 'lib/core/shell/panel_tabs.dart';
import 'lib/core/session/session_controller.dart';
import 'lib/core/storage/token_store.dart';
import 'lib/features/maps/domain/map_state.dart';
Future<void> main()async{
 WidgetsFlutterBinding.ensureInitialized();await initFormatting();
 final prefs=await SharedPreferences.getInstance();
 final c=ProviderContainer(overrides:[sharedPreferencesProvider.overrideWithValue(prefs),tokenStoreProvider.overrideWithValue(MemoryTokenStore(Platform.environment['BIO_QA_TOKEN']))],retry:(_,_)=>null);
 await c.read(sessionProvider.notifier).restore();
 final regions=await c.read(regionsApiProvider).list();
 if(regions.isNotEmpty)c.read(selectedRegionProvider.notifier).select(regions.firstWhere((r)=>r.kind.name=='province',orElse:()=>regions.first));
 runApp(UncontrolledProviderScope(container:c,child:const EcoApp()));
 WidgetsBinding.instance.addPostFrameCallback((_){c.read(routerProvider).go(Platform.environment['BIO_QA_ROUTE']??'/');final tab=Platform.environment['BIO_QA_TAB'];if(tab!=null&&tab.isNotEmpty)c.read(panelTabProvider.notifier).select(tab);});
}
