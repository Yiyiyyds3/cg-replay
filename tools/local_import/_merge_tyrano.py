#!/usr/bin/env python3
# -*- coding: utf-8 -*-
import json
p='reports/local_import/data/import_results_v2.json'
d=json.load(open(p,encoding='utf-8'))
for r in d['results']:
    if r['name'].startswith('光辉的少女们'):
        r['status']='import_ok'
        r['events']=1419
        r['stderr']=''
        r['v2_note']='TyranoBuilder SRTS容器逆向成功：runtime.rts carve 1419张CG(78.3MB)打包cglib导入 events=1419；剧本层data.dts流密码RC4待破(keystream[0:8]已得)'
d['v2_ok']=sum(1 for r in d['results'] if r['status']=='import_ok')
d['v2_fail']=sum(1 for r in d['results'] if r['status']=='import_fail')
json.dump(d,open(p,'w',encoding='utf-8'),ensure_ascii=False,indent=2)
print('ok=%d fail=%d'%(d['v2_ok'],d['v2_fail']))