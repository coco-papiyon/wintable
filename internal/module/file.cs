public partial class Receiver {
 [DllImport("kernel32",CharSet=CharSet.Unicode,SetLastError=true)]
 static extern SafeFileHandle CreateFile(string p,uint a,uint s,IntPtr sa,uint mode,uint flags,IntPtr tpl);
 [DllImport("kernel32",CharSet=CharSet.Unicode,SetLastError=true)]
 static extern bool CreateDirectory(string p,IntPtr sa);
 [DllImport("kernel32")]
 static extern bool GetFileInformationByHandle(SafeFileHandle h,[Out] byte[] b);
 static SafeFileHandle Guard(string p){var h=CreateFile(p,0,3,IntPtr.Zero,3,0x02200000,IntPtr.Zero);var b=new byte[52];if(h.IsInvalid || !GetFileInformationByHandle(h,b) || (BitConverter.ToUInt32(b,0)&0x410)!=0x10){h.Dispose();throw new Exception();}return h;}
 public static void Run(){string r="{\"error\":\"invalid_request\"}";var guards=new List<SafeFileHandle>();try{
 byte[] x=Read(Console.OpenStandardInput(),2097153);if(x.Length>2097152)throw new Exception();var p=new Receiver();p.s=new UTF8Encoding(false,true).GetString(x);var d=(Dictionary<string,object>)p.V(0);p.W();if(p.i!=p.s.Length || d.Count!=2 || (string)d["module"]!="file")throw new Exception();var a=(Dictionary<string,object>)d["args"];if(a.Count!=2 || (string)a["state"]!="directory")throw new Exception();string path=(string)a["path"];
 if(!System.Text.RegularExpressions.Regex.IsMatch(path,@"\A[A-Za-z]:\\[^\x00-\x1f\x7f-\x9f\""<>:|?*/]+\z"))throw new Exception();
 foreach(string c in path.Substring(3).Split('\\')){string name=c.Split('.')[0];if(c.Length==0 || c=="." || c==".." || c.EndsWith(".") || c.EndsWith(" ") || System.Text.RegularExpressions.Regex.IsMatch(name,@"\A(CON|PRN|AUX|NUL|COM[1-9¹²³]|LPT[1-9¹²³])\z",System.Text.RegularExpressions.RegexOptions.IgnoreCase))throw new Exception();}
 r="{\"error\":\"execution_failed\"}";string current=path.Substring(0,3);guards.Add(Guard(current));bool changed=false;
 foreach(string c in path.Substring(3).Split('\\')){current=Path.Combine(current,c);if(CreateDirectory(current,IntPtr.Zero))changed=true;else if(Marshal.GetLastWin32Error()!=183)throw new Exception();guards.Add(Guard(current));}
 r=changed?"{\"changed\":true}":"{\"changed\":false}";
 }catch{}finally{foreach(var h in guards)h.Dispose();}byte[] output=Encoding.UTF8.GetBytes(r);Console.OpenStandardOutput().Write(output,0,output.Length);
 }
}
