using System.Runtime.InteropServices;
using Microsoft.Win32.SafeHandles;
using System;
using System.IO;
using System.Text;
using System.Collections.Generic;
public partial class Receiver {
 string s; int i;
 void W(){while(i<s.Length && " \r\n\t".IndexOf(s[i])>=0)i++;}
 char C(){if(i==s.Length)throw new Exception();return s[i++];}
 string S(){if(C()!='"')throw new Exception();var b=new StringBuilder();for(;;){char c=C();if(c=='"')break;if(c<32)throw new Exception();if(c=='\\'){c=C();int n="\"\\/bfnrtu".IndexOf(c);if(n<0)throw new Exception();if(n==8){string hex=s.Substring(i,4);foreach(char h in hex)if(!Uri.IsHexDigit(h))throw new Exception();c=(char)Convert.ToInt32(hex,16);i+=4;}else c="\"\\/\b\f\n\r\t"[n];}b.Append(c);}string v=b.ToString();new UTF8Encoding(false,true).GetBytes(v);return v;}
 object V(int depth){if(depth>3)throw new Exception();W();char c=C();i--;if(c=='"')return S();if(c=='['){i++;var a=new List<string>();W();if(s[i]!=']')for(;;){W();a.Add(S());W();c=C();if(c==']')return a;if(c!=',')throw new Exception();}i++;return a;}if(c!='{')throw new Exception();i++;var d=new Dictionary<string,object>(StringComparer.Ordinal);W();if(s[i]!='}')for(;;){W();string k=S();W();if(C()!=':')throw new Exception();d.Add(k,V(depth+1));W();c=C();if(c=='}')return d;if(c!=',')throw new Exception();}i++;return d;}
 static byte[] Read(Stream s,int max){using(var m=new MemoryStream()){byte[] b=new byte[8192];while(m.Length<max){int n=s.Read(b,0,(int)Math.Min(b.Length,max-m.Length));if(n==0)break;m.Write(b,0,n);}return m.ToArray();}}
}
