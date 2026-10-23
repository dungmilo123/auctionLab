"""Deterministic project-authored vector object studies, CC0-1.0. No raster generation."""
from pathlib import Path
import json
root=next(p for p in Path(__file__).resolve().parents if (p/'go.mod').is_file() and (p/'web').is_dir())
records=json.loads((root/'internal/catalog/seed_data.json').read_text())
assets=root/'web/public/site-assets/catalog'
# Shared materials and neutral raking light, with distinct silhouettes and details.
defs='''<defs>
<linearGradient id="paper" x2="0.2" y2="1"><stop stop-color="#f7f2e9"/><stop offset="1" stop-color="#ded2c0"/></linearGradient>
<linearGradient id="metal" x2="0.15" y2="1"><stop stop-color="#f3eee5"/><stop offset=".2" stop-color="#aaa8a0"/><stop offset=".45" stop-color="#e1dfd6"/><stop offset=".75" stop-color="#8c8980"/><stop offset="1" stop-color="#d5cdbf"/></linearGradient>
<linearGradient id="brass" x2=".6" y2="1"><stop stop-color="#d3b47e"/><stop offset=".4" stop-color="#9a763d"/><stop offset=".65" stop-color="#e1c690"/><stop offset="1" stop-color="#705332"/></linearGradient>
<linearGradient id="wood" x2="1" y2=".4"><stop stop-color="#48352a"/><stop offset=".4" stop-color="#8b6142"/><stop offset="1" stop-color="#51362b"/></linearGradient>
<linearGradient id="leather" x2=".3" y2="1"><stop stop-color="#4b4941"/><stop offset="1" stop-color="#1f2221"/></linearGradient>
<radialGradient id="glass"><stop stop-color="#b4bdad"/><stop offset=".3" stop-color="#344a49"/><stop offset=".64" stop-color="#172c32"/><stop offset="1" stop-color="#0c1519"/></radialGradient>
<radialGradient id="vinyl"><stop stop-color="#414747"/><stop offset="1" stop-color="#111b20"/></radialGradient>
<filter id="shadow" x="-50%" y="-80%" width="200%" height="260%"><feGaussianBlur stdDeviation="9"/></filter>
<pattern id="grain" width="14" height="9" patternUnits="userSpaceOnUse"><path d="M0 2h14M0 7h14" stroke="#e9d1a8" stroke-width=".35" opacity=".2"/></pattern>
<pattern id="cloth" width="5" height="5" patternUnits="userSpaceOnUse"><path d="M0 0h5M0 0v5" stroke="#989282" stroke-width=".7" opacity=".25"/></pattern>
</defs>'''
def lens(x=325,y=268,r=72):
 return f'<circle cx="{x}" cy="{y}" r="{r+10}" fill="url(#metal)" stroke="#3a3832" stroke-width="3"/><circle cx="{x}" cy="{y}" r="{r}" fill="#252b2a"/><circle cx="{x}" cy="{y}" r="{r-8}" fill="none" stroke="#8b8a7f" stroke-width="2"/><circle cx="{x}" cy="{y}" r="{r-16}" fill="url(#glass)"/><path d="M{x-r//2} {y-r//3}q{r//3} {-r//2} {r} -8" fill="none" stroke="#dedfce" stroke-width="5" opacity=".22"/>'
def ticks(cx,cy,r,n=60):
 return ''.join(f'<path d="M{cx} {cy-r}v{9 if i%5==0 else 4}" transform="rotate({i*360/n} {cx} {cy})" stroke="#413c34" stroke-width="{2 if i%5==0 else 1}"/>' for i in range(n))
def dial(cx=320,cy=250,r=70):
 return f'<circle cx="{cx}" cy="{cy}" r="{r+9}" fill="url(#brass)"/><circle cx="{cx}" cy="{cy}" r="{r}" fill="#eee5d4"/>'+ticks(cx,cy,r-6)+f'<path d="M{cx} {cy-r*.52}V{cy}l{r*.55} {r*.3}" fill="none" stroke="#34362f" stroke-width="5"/><circle cx="{cx}" cy="{cy}" r="5" fill="#766247"/>'
objects=[]
# Folding camera: bellows, rails and handle.
objects.append('<path d="M180 145q-25-55 165-20" fill="none" stroke="#684f3c" stroke-width="12"/><rect x="155" y="155" width="315" height="176" rx="8" fill="url(#leather)" stroke="#928878" stroke-width="3"/><path d="M192 180h130v135H192z" fill="#503c30"/>'+''.join(f'<path d="M{210+i*10} 185l-8 115h15l8-115" fill="#2b2622" stroke="#75614e" stroke-width="1"/>' for i in range(8))+'<path d="M185 328h280l-15 26H175z" fill="url(#metal)"/><rect x="340" y="163" width="88" height="153" rx="4" fill="url(#metal)"/>'+lens(382,244,49)+'<circle cx="444" cy="183" r="8" fill="url(#brass)"/>')
# Rangefinder.
objects.append('<rect x="145" y="162" width="350" height="195" rx="12" fill="url(#leather)"/><path d="M145 211v-40q0-16 16-16h318q16 0 16 16v40zM145 333h350v20H145z" fill="url(#metal)"/><rect x="173" y="174" width="42" height="22" rx="3" fill="#344346"/><rect x="240" y="174" width="30" height="22" fill="#555b53"/><rect x="420" y="143" width="38" height="16" rx="3" fill="url(#metal)"/>'+lens(336,273,68)+'<path d="M160 228v90M477 228v90" stroke="#777c71" stroke-width="2"/>')
# Twin lens: tall leather housing.
objects.append('<path d="M242 125l20-28h116l18 28v247H242z" fill="url(#leather)" stroke="#7e796d" stroke-width="3"/><rect x="264" y="130" width="112" height="224" rx="4" fill="url(#metal)"/>'+lens(320,192,36)+lens(320,293,46)+'<circle cx="228" cy="254" r="16" fill="url(#metal)"/><circle cx="399" cy="295" r="14" fill="url(#metal)"/><path d="M275 111h90" stroke="#b9b3a5" stroke-width="3"/>')
# Compact camera.
objects.append('<rect x="163" y="187" width="320" height="165" rx="22" fill="#a8aaa1" stroke="#777c72" stroke-width="2"/><path d="M164 250h318v81q0 22-22 22H185q-22 0-22-22z" fill="url(#leather)"/><rect x="185" y="204" width="75" height="26" rx="3" fill="#e0dac8"/><path d="M195 207v20m8-20v20m8-20v20m8-20v20m8-20v20m8-20v20" stroke="#aaa68f"/><rect x="382" y="205" width="48" height="20" rx="2" fill="#31413e"/>'+lens(326,278,51)+'<rect x="400" y="179" width="38" height="10" rx="4" fill="#4b5149"/>')
# Leather album.
objects.append('<path d="M191 122l266 24-18 237-267-24z" fill="#b7a382"/><path d="M177 105l284 26-18 245-284-26z" fill="#684b37" stroke="#3f3026" stroke-width="5"/><path d="M198 110l-18 243" stroke="#ac8961" stroke-width="3"/><path d="M218 151l207 19-12 151-207-19z" fill="none" stroke="#a28760" stroke-width="2"/><path d="M237 186l150 14-8 89-150-14z" fill="#c6b89b"/><path d="M232 262l45-42 34 28 34-33 35 64" fill="#8f9480"/><circle cx="352" cy="226" r="10" fill="#e3d6b7"/>')
# Ceramic frame.
objects.append('<path d="M216 109h209v274H216z" fill="#d4c9b1" stroke="#aaa28f" stroke-width="4"/><rect x="236" y="130" width="169" height="232" rx="3" fill="#f4eddf" stroke="#7e8779" stroke-width="5"/><rect x="251" y="146" width="138" height="200" fill="#9eaa9e"/><path d="M251 292l46-65 37 32 55-83v170H251z" fill="#66776d"/><path d="M263 127h128M232 154v192" stroke="#fff8e6" stroke-width="3"/><circle cx="299" cy="195" r="16" fill="#e4d9b8"/>')
# Canvas field watch.
objects.append('<path d="M285 70h70l11 360h-92z" fill="#737963"/><path d="M285 70h70l11 360h-92z" fill="url(#cloth)"/><path d="M292 75l-8 349m63-349 8 349" stroke="#c1b798" stroke-dasharray="3 4"/>'+dial(320,250,71)+'<rect x="399" y="238" width="14" height="25" rx="4" fill="url(#metal)"/><path d="M303 398h35" stroke="#272f2a" stroke-width="3"/>')
# Pocket watch.
objects.append('<ellipse cx="320" cy="115" rx="21" ry="25" fill="none" stroke="url(#brass)" stroke-width="8"/><rect x="307" y="130" width="26" height="23" rx="5" fill="url(#brass)"/>'+dial(320,256,99)+'<circle cx="320" cy="299" r="21" fill="none" stroke="#8e8068"/><path d="M320 285v15l11 5" fill="none" stroke="#605b4d" stroke-width="2"/>')
# Walnut desk clock.
objects.append('<path d="M192 342V165q0-24 24-24h208q24 0 24 24v177z" fill="url(#wood)" stroke="#493a2d" stroke-width="4"/><path d="M192 342V165q0-24 24-24h208q24 0 24 24v177z" fill="url(#grain)"/><path d="M180 342h280v22H180z" fill="#493a2d"/>'+dial(320,246,78))
# Brass vase.
objects.append('<path d="M293 112h54l-8 90q-2 16 18 46 51 91 20 117H263q-32-26 20-117 20-30 18-46z" fill="url(#brass)" stroke="#a18659" stroke-width="2"/><ellipse cx="320" cy="112" rx="27" ry="7" fill="#74623d"/><path d="M303 224q-47 96-27 128" fill="none" stroke="#edd8a5" stroke-width="7" opacity=".6"/><ellipse cx="320" cy="365" rx="57" ry="9" fill="#6e5734"/>')
# Mantel clock.
objects.append('<path d="M175 345V217q0-31 34-42l48-19q63-98 126 0l48 19q34 11 34 42v128z" fill="url(#wood)" stroke="#483529" stroke-width="4"/><path d="M175 345V217q0-31 34-42l48-19q63-98 126 0l48 19q34 11 34 42v128z" fill="url(#grain)"/>'+dial(320,237,72)+'<path d="M160 346h320v23H160z" fill="#513a2d"/><path d="M196 369v9m248-9v9" stroke="#242722" stroke-width="10"/>')
# Travel watch rectangular face.
objects.append('<path d="M287 82h66l8 338h-82z" fill="#704e38"/><path d="M291 88l-5 325m60-325 7 325" stroke="#b89e74" stroke-dasharray="3 5"/><rect x="247" y="170" width="146" height="165" rx="18" fill="url(#metal)" stroke="#b4aa96" stroke-width="2"/><rect x="260" y="183" width="120" height="139" rx="10" fill="#e4d7bc"/>'+ticks(320,253,51,12)+'<path d="M320 217v36l31 19" fill="none" stroke="#453b30" stroke-width="4"/><circle cx="320" cy="253" r="4" fill="#876847"/><rect x="395" y="241" width="12" height="22" rx="3" fill="url(#metal)"/>')
# Three distinct records and sleeves, deliberately no invented performer text.
for color,shape in [('#344f5a','jazz'),('#858269','chamber'),('#a26c4a','soul')]:
 art=('<path d="M163 162h28v130h-28zM215 196h22v96h-22zM261 135h17v157h-17z" fill="#c7b985"/>' if shape=='jazz' else '<path d="M160 286q15-123 57-93t48 61" fill="none" stroke="#ded2af" stroke-width="8"/>' if shape=='chamber' else '<circle cx="215" cy="237" r="66" fill="#d4b079"/><circle cx="215" cy="237" r="42" fill="#76584a"/>')
 objects.append(f'<rect x="131" y="129" width="250" height="250" fill="{color}" stroke="#c9bda4" stroke-width="2"/>'+art+'<circle cx="379" cy="255" r="130" fill="url(#vinyl)"/>'+''.join(f'<circle cx="379" cy="255" r="{r}" fill="none" stroke="#6f7772" stroke-width=".5" opacity=".45"/>' for r in range(51,126,5))+f'<circle cx="379" cy="255" r="44" fill="{color}"/><circle cx="379" cy="255" r="4" fill="#d8cbb0"/>')
# Stereo receiver, turntable and speakers.
objects.append('<path d="M115 185l36-25h380l-25 25z" fill="#695646"/><rect x="115" y="185" width="391" height="164" rx="4" fill="url(#metal)" stroke="#6d7067" stroke-width="3"/><rect x="136" y="204" width="349" height="59" rx="2" fill="#233639"/>'+''.join(f'<path d="M{152+i*9} 222v{14 if i%5 else 23}" stroke="#a8b7a0" stroke-width="1"/>' for i in range(35))+'<path d="M155 244h310" stroke="#9a4b3c" stroke-width="2"/>'+''.join(f'<circle cx="{165+i*54}" cy="304" r="{20 if i<5 else 28}" fill="url(#metal)" stroke="#6f746b" stroke-width="2"/><path d="M{165+i*54} 288v8" stroke="#555a50"/>' for i in range(6))+'<path d="M132 350v13m354-13v13" stroke="#363b34" stroke-width="12"/>')
objects.append('<path d="M129 190l48-51h329l-24 51z" fill="#6e5037"/><path d="M129 190h353v147H129z" fill="url(#wood)"/><path d="M129 190h353v147H129z" fill="url(#grain)"/><path d="M143 184l38-32h309l-18 32z" fill="url(#metal)"/><ellipse cx="292" cy="190" rx="111" ry="39" fill="#1c2524"/>'+''.join(f'<ellipse cx="292" cy="190" rx="{r}" ry="{r*.35}" fill="none" stroke="#7c8278" stroke-width=".7"/>' for r in range(40,107,8))+'<ellipse cx="292" cy="190" rx="32" ry="12" fill="#907b52"/><path d="M292 179v12" stroke="#d7d0bf" stroke-width="3"/><path d="M438 169v24l-24 27-42 7" fill="none" stroke="#afb4a8" stroke-width="7"/><rect x="362" y="224" width="21" height="8" fill="#323b35"/><circle cx="449" cy="276" r="16" fill="url(#metal)"/>')
objects.append(''.join(f'<g transform="translate({dx} {dy})"><path d="M0 0l27-14h123l-12 14z" fill="#956f49"/><path d="M138 0l12-14v243l-12 12z" fill="#49382a"/><rect width="138" height="241" fill="url(#wood)"/><rect width="138" height="241" fill="url(#grain)"/><rect x="12" y="12" width="114" height="217" fill="#454840"/><circle cx="69" cy="58" r="24" fill="#252e2d" stroke="#8b8d7c" stroke-width="3"/><circle cx="69" cy="151" r="47" fill="#222b2b" stroke="#888674" stroke-width="4"/><circle cx="69" cy="151" r="30" fill="#4e5550"/><circle cx="69" cy="151" r="14" fill="#2d3734"/></g>' for dx,dy in [(159,133),(337,112)]))
assert len(objects)==18
for i,(r,art) in enumerate(zip(records,objects),1):
 for detail in [False,True]:
  view='160 110 320 250' if detail else '0 0 640 500'
  # Detail is a close inspection of the same authored geometry, not a different object.
  svg=f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="{view}" width="640" height="500" role="img" aria-labelledby="title desc"><title id="title">{r["title"]}{", detail" if detail else ""}</title><desc id="desc">Project-authored vector object study with neutral light and material detail.</desc>{defs}<rect width="640" height="500" fill="url(#paper)"/><ellipse cx="320" cy="387" rx="175" ry="14" fill="#51452e" opacity=".16" filter="url(#shadow)"/>{art}</svg>'
  (assets/f'lot-{i:02d}{"-detail" if detail else ""}.svg').write_text(svg)
for n,(title,ids) in enumerate([('Analog Memories',[0,1,4]),('Objects of Time',[7,9,10]),('The Listening Room',[12,16,17])],1):
 art=''.join(f'<g transform="translate({x} {y}) scale({scale})">{objects[i]}</g>' for i,x,y,scale in [(ids[0],-30,95,.65),(ids[1],245,-10,.85),(ids[2],85,230,.55)])
 (assets/f'event-{n}.svg').write_text(f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 800 600" width="800" height="600" role="img" aria-labelledby="title desc"><title id="title">{title} collection</title><desc id="desc">A composed collection of related project-authored object studies.</desc>{defs}<rect width="800" height="600" fill="url(#paper)"/><path d="M0 490h800" stroke="#d3c6b1"/>{art}</svg>')
(assets/'README.md').write_text('''# Catalog object studies

All 39 SVGs are project-authored vector artwork: 18 distinct object plates,
18 close detail studies of the same objects, and three composed collection covers.
Source: deterministic geometry/material gradients in Change 9 verification/artwork.py.
Rights: CC0-1.0. Generated raster/AI imagery: none. No third-party photographs,
marks, remote fetches, embedded fonts or authentication evidence. These studies
illustrate catalog subjects; they do not certify physical condition or provenance.
Existing /site-assets/catalog/*.svg URLs and image order remain compatible.
Titles describe objects without baked-in demonstration lettering. The existing
/site-assets/image-unavailable.svg remains the local failure fallback.

Manifest: lot-01 through lot-18.svg (object), the same stems with -detail.svg
(detail), event-1.svg (Analog Memories), event-2.svg (Objects of Time), event-3.svg
(The Listening Room). This accounts for all 39 distributed catalog assets.
''')
print('Wrote 39 local object/detail/collection studies')
