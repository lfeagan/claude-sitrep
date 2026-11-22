#!/usr/bin/env python3
"""Turns the .ans frames of TestWriteThemeShots into PNG sheets: each sheet stacks the frames as a person with normal sight, then with
protanopia, deuteranopia and tritanopia (Machado et al. 2009, severity 1.0) would see them. Needs only Python 3 and rsvg-convert."""
import re, subprocess, sys, unicodedata, os
from xml.sax.saxutils import escape

CW, CH, FS = 9, 18, 15
CVD = {
    "normal": None,
    "protanopia (no red)": [[0.152286, 1.052583, -0.204868], [0.114503, 0.786281, 0.099216], [-0.003882, -0.048116, 1.051998]],
    "deuteranopia (no green)": [[0.367322, 0.860646, -0.227968], [0.280085, 0.672501, 0.047413], [-0.011820, 0.042940, 0.968881]],
    "tritanopia (no blue)": [[1.255528, -0.076749, -0.178779], [-0.078411, 0.930809, 0.147602], [0.004733, 0.691367, 0.303900]],
}
if os.environ.get("CVD_ONLY"):
    CVD = {k: v for k, v in CVD.items() if k.startswith(os.environ["CVD_ONLY"])}
def lin(c): c/=255; return c/12.92 if c<=0.04045 else ((c+0.055)/1.055)**2.4
def unlin(c): c=max(0,min(1,c)); return round(255*(12.92*c if c<=0.0031308 else 1.055*c**(1/2.4)-0.055))
def cvd(rgb, m):
    if m is None: return rgb
    l=[lin(x) for x in rgb]
    return tuple(unlin(sum(m[i][j]*l[j] for j in range(3))) for i in range(3))
def hexs(rgb): return "#%02x%02x%02x"%rgb
def xterm(n):
    if n<8: return tuple(170*((n>>b)&1) for b in (0,1,2))
    if n<16: return tuple(170*((n>>b)&1)+85 for b in (0,1,2))
    if n<232:
        n-=16; lv=[0,95,135,175,215,255]; return (lv[n//36],lv[n//6%6],lv[n%6])
    v=8+10*(n-232); return (v,v,v)
def width(ch):
    if unicodedata.combining(ch): return 0
    return 2 if unicodedata.east_asian_width(ch) in "WF" else 1

SGR=re.compile(r"\x1b\[([0-9;]*)m")
def parse(line, dfg, dbg):
    cells=[]; fg,bg,bold,ul,rev,faint=None,None,False,False,False,False; pos=0
    def put(text):
        for ch in text:
            w=width(ch)
            if w==0: continue
            cells.append((ch,w,fg,bg,bold,ul,rev,faint))
    for m in SGR.finditer(line):
        put(line[pos:m.start()]); pos=m.end()
        p=[int(x) if x else 0 for x in m.group(1).split(";")] if m.group(1) else [0]; i=0
        while i<len(p):
            c=p[i]
            if c==0: fg,bg,bold,ul,rev,faint=None,None,False,False,False,False
            elif c==1: bold=True
            elif c==2: faint=True
            elif c==4: ul=True
            elif c==7: rev=True
            elif c==22: bold=faint=False
            elif c==24: ul=False
            elif c==27: rev=False
            elif 30<=c<=37: fg=xterm(c-30)
            elif 90<=c<=97: fg=xterm(c-90+8)
            elif 40<=c<=47: bg=xterm(c-40)
            elif 100<=c<=107: bg=xterm(c-100+8)
            elif c==39: fg=None
            elif c==49: bg=None
            elif c in (38,48):
                if p[i+1]==5: col=xterm(p[i+2]); i+=2
                else: col=tuple(p[i+2:i+5]); i+=4
                if c==38: fg=col
                else: bg=col
            i+=1
    put(line[pos:]); return cells

def sheet(path, out):
    side="dark" if path.endswith("-dark.ans") else "light"
    dfg,dbg=((230,230,230),(30,30,30)) if side=="dark" else ((26,26,26),(255,255,255))
    frames=open(path,encoding="utf-8").read().split("\n=====\n")
    lines_per=[f.rstrip("\n").split("\n") for f in frames]
    body=[]; y=0; maxw=100
    parts=[]
    for label,m in CVD.items():
        parts.append(f'<rect x="0" y="{y}" width="{(maxw+2)*CW}" height="{CH+4}" fill="{hexs(cvd((90,90,90),m))}"/>')
        parts.append(f'<text x="6" y="{y+CH-4}" font-family="Menlo" font-size="13" font-weight="bold" fill="#ffffff">{escape(os.path.basename(path)[:-4])} - {escape(label)}</text>')
        y+=CH+4
        for fi,lines in enumerate(lines_per):
            for line in lines:
                cells=parse(line,dfg,dbg); x=0
                parts.append(f'<rect x="0" y="{y}" width="{(maxw+2)*CW}" height="{CH}" fill="{hexs(cvd(dbg,m))}"/>')
                # runs of equal style
                run=[]; 
                def flush(run,x0):
                    if not run: return
                    ch0=run[0]; text="".join(c[0] for c in run); cols=sum(c[1] for c in run)
                    _,_,fg,bg,bold,ul,rev,faint=ch0
                    f=fg or dfg; b=bg or dbg
                    if rev: f,b=(bg or dbg),(fg or dfg)
                    if faint: f=tuple((a+bb)//2 for a,bb in zip(f,b))
                    f,b=cvd(f,m),cvd(b,m)
                    if b!=cvd(dbg,m): parts.append(f'<rect x="{x0*CW}" y="{y}" width="{cols*CW}" height="{CH}" fill="{hexs(b)}"/>')
                    if text.strip():
                        parts.append(f'<text x="{x0*CW}" y="{y+CH-5}" font-family="Menlo" font-size="{FS}" textLength="{cols*CW}" lengthAdjust="spacing" xml:space="preserve" fill="{hexs(f)}"'+(' font-weight="bold"' if bold else '')+(' text-decoration="underline"' if ul else '')+f'>{escape(text)}</text>')
                key=None
                for c in cells:
                    k=c[2:]
                    if key is not None and k!=key: flush(run,x-sum(r[1] for r in run)); run=[]
                    key=k; run.append(c); x+=c[1]
                flush(run,x-sum(r[1] for r in run))
                y+=CH
            y+=6
        y+=10
    svg=f'<svg xmlns="http://www.w3.org/2000/svg" width="{(maxw+2)*CW}" height="{y}" viewBox="0 0 {(maxw+2)*CW} {y}"><rect width="100%" height="100%" fill="#444"/>'+"".join(parts)+"</svg>"
    open(out+".svg","w").write(svg)
    subprocess.run(["rsvg-convert","-o",out,out+".svg"],check=True)
    os.remove(out+".svg")

if __name__=="__main__":
    src,dst=sys.argv[1],sys.argv[2]; os.makedirs(dst,exist_ok=True)
    for f in sorted(os.listdir(src)):
        if f.endswith(".ans"): sheet(os.path.join(src,f), os.path.join(dst,f[:-4]+".png")); print(f[:-4]+".png")
