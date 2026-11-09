// The CLR argument parser is deliberately unused. msvcrt.dll parses the actual
// GetCommandLineW buffer through its native __wgetmainargs CRT implementation.
using System;
using System.IO;
using System.Runtime.InteropServices;
using System.Text;
using System.Threading.Tasks;
public class CRTReceiver {
 [DllImport("msvcrt.dll", CallingConvention=CallingConvention.Cdecl)]
 static extern int __wgetmainargs(out int count, out IntPtr args, out IntPtr env, int glob, ref int startup);
 public static int Main() {
  int count, startup=0;IntPtr args,env;
  if(__wgetmainargs(out count,out args,out env,0,ref startup)!=0)return 99;
  string[] values=new string[count-1];
  for(int i=1;i<count;i++)values[i-1]=Marshal.PtrToStringUni(Marshal.ReadIntPtr(args,i*IntPtr.Size));
  if(values.Length>0 && values[0]=="--stdin-eof")return Console.OpenStandardInput().ReadByte()==-1?0:97;
  if(values.Length>0 && values[0]=="--bytes") {
   int n=Int32.Parse(values[1]);byte[] data=new byte[n];
   var task=Task.Factory.StartNew(()=>Console.OpenStandardError().Write(data,0,n));
   Console.OpenStandardOutput().Write(data,0,n);task.Wait();return 0;
  }
  if(values.Length>0 && (values[0]=="--tree" || values[0]=="--orphan")) {
   var child=System.Diagnostics.Process.Start(new System.Diagnostics.ProcessStartInfo(System.Diagnostics.Process.GetCurrentProcess().MainModule.FileName,"--wait \""+values[1]+".started\""){UseShellExecute=false,CreateNoWindow=true});
   File.WriteAllText(values[1],child.Id.ToString());
   if(values[0]=="--tree")System.Threading.Thread.Sleep(60000);
   return 0;
  }
  if(values.Length>0 && values[0]=="--marker") { File.WriteAllText(values[1],"executed");return 0; }
  if(values.Length>0 && values[0]=="--wait") { File.AppendAllText(values[1],"started\n");System.Threading.Thread.Sleep(60000);return 0; }
  if(values.Length>0 && values[0]=="--assert-absent")return File.Exists(values[1])?98:0;
  if(values.Length>0 && values[0]=="--exit")return Int32.Parse(values[1]);
  if(values.Length>0 && values[0]=="--codepage") {
   int cp=Int32.Parse(values[1]);byte[] data=Encoding.GetEncoding(cp).GetBytes(cp==932||cp==65001?"日本語":"caf\u00e9");
   var stream=Console.OpenStandardOutput();stream.Write(data,0,data.Length);return 0;
  }
  if(values.Length>0 && values[0]=="--flood") {
   byte[] b=new byte[8192];for(int i=0;i<b.Length;i++)b[i]=(byte)(i%256);
   var t=Task.Factory.StartNew(()=>{var e=Console.OpenStandardError();for(int i=0;i<130;i++)e.Write(b,0,b.Length);});
   var o=Console.OpenStandardOutput();for(int i=0;i<130;i++)o.Write(b,0,b.Length);t.Wait();return 7;
  }
  var output=Console.OpenStandardOutput();
  foreach(string s in values) { byte[] b=Encoding.ASCII.GetBytes(Convert.ToBase64String(Encoding.Unicode.GetBytes(s))+"\n");output.Write(b,0,b.Length); }
  return 0;
 }
}
