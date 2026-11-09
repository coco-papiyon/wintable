public partial class Receiver {
 [DllImport("kernel32",CharSet=CharSet.Unicode,SetLastError=true)]
 static extern SafeFileHandle CreateFile(string p,uint a,uint s,IntPtr sa,uint mode,uint flags,IntPtr tpl);
 [DllImport("kernel32",SetLastError=true)]
 static extern bool GetFileInformationByHandle(SafeFileHandle h,[Out] byte[] b);
 static byte[] Info(SafeFileHandle h,bool dir){var b=new byte[52];if(h.IsInvalid || !GetFileInformationByHandle(h,b) || (BitConverter.ToUInt32(b,0)&0x410)!=(dir?16u:0u) || (!dir && BitConverter.ToUInt32(b,40)!=1))throw new Exception();return b;}
 static SafeFileHandle Open(string p,bool dir){var h=CreateFile(p,dir?0u:0x80000000u,dir?3u:1u,IntPtr.Zero,3,0x02200000,IntPtr.Zero);try{Info(h,dir);return h;}catch{h.Dispose();throw;}}
 [StructLayout(LayoutKind.Sequential)]struct US{public ushort Length,Maximum;public IntPtr Text;}
 [StructLayout(LayoutKind.Sequential)]struct OA{public int Length;public IntPtr Root,Name;public uint Flags;public IntPtr SD,QOS;}
 [StructLayout(LayoutKind.Sequential)]struct IOS{public IntPtr Status,Information;}
 [DllImport("ntdll")]static extern int NtCreateFile(out SafeFileHandle h,uint access,ref OA oa,out IOS ios,IntPtr size,uint attrs,uint share,uint disposition,uint options,IntPtr ea,uint ealen);
 static SafeFileHandle Child(SafeFileHandle parent,string name,bool dir){
 IntPtr text=Marshal.StringToHGlobalUni(name),ptr=Marshal.AllocHGlobal(Marshal.SizeOf(typeof(US)));try{
 var u=new US{Length=checked((ushort)(name.Length*2)),Maximum=checked((ushort)(name.Length*2+2)),Text=text};Marshal.StructureToPtr(u,ptr,false);var oa=new OA{Length=Marshal.SizeOf(typeof(OA)),Root=parent.DangerousGetHandle(),Name=ptr,Flags=64};SafeFileHandle h;IOS ios;
 int status=NtCreateFile(out h,dir?0x1000a0u:0x80100000u,ref oa,out ios,IntPtr.Zero,0,dir?3u:1u,1,0x200020u|(dir?1u:64u),IntPtr.Zero,0);if(status<0){if(h!=null)h.Dispose();throw new Exception();}try{Info(h,dir);return h;}catch{h.Dispose();throw;}
 }finally{Marshal.FreeHGlobal(ptr);Marshal.FreeHGlobal(text);}}
 static byte[] Hash(FileStream f){f.Position=0;using(var h=System.Security.Cryptography.SHA256.Create())return h.ComputeHash(f);}
 static bool Same(byte[] a,byte[] b){if(a.Length!=b.Length)return false;for(int j=0;j<a.Length;j++)if(a[j]!=b[j])return false;return true;}
 static bool Stable(byte[] a,byte[] b){for(int j=0;j<a.Length;j++)if((j<12 || j>=20) && a[j]!=b[j])return false;return true;}
 public static void Run(){var guards=new List<SafeFileHandle>();try{
 byte[] x=Read(Console.OpenStandardInput(),2097153);if(x.Length>2097152)throw new Exception();var p=new Receiver();p.s=new UTF8Encoding(false,true).GetString(x);var d=(Dictionary<string,object>)p.V(0);p.W();if(p.i!=p.s.Length || d.Count!=2 || (string)d["module"]!="fetch")throw new Exception();var a=(Dictionary<string,object>)d["args"];if(a.Count!=1)throw new Exception();string path=(string)a["src"];
 if(!System.Text.RegularExpressions.Regex.IsMatch(path,@"\A[A-Za-z]:\\[^\x00-\x1f\x7f-\x9f\""<>:|?*/]+\z"))throw new Exception();foreach(string c in path.Substring(3).Split('\\')){if(c.Length==0 || c.EndsWith(".") || c.EndsWith(" ") || System.Text.RegularExpressions.Regex.IsMatch(c.Split('.')[0],@"\A(CON|PRN|AUX|NUL|COM[1-9]|LPT[1-9])\z",System.Text.RegularExpressions.RegexOptions.IgnoreCase))throw new Exception();}
 guards.Add(Open(path.Substring(0,3),true));string[] parts=path.Substring(3).Split('\\');for(int j=0;j<parts.Length-1;j++)guards.Add(Child(guards[guards.Count-1],parts[j],true));
 using(var h=Child(guards[guards.Count-1],parts[parts.Length-1],false))using(var f=new FileStream(h,FileAccess.Read)){
 byte[] info=Info(h,false);long size=f.Length;if(size>1073741824)throw new Exception();byte[] before=Hash(f);f.Position=0;var output=Console.OpenStandardOutput();byte[] header=Encoding.ASCII.GetBytes("WTF1");output.Write(header,0,4);byte[] n=BitConverter.GetBytes(size);output.Write(n,0,8);
 using(var hash=System.Security.Cryptography.SHA256.Create()){byte[] buf=new byte[65536];long total=0;for(;;){int count=f.Read(buf,0,buf.Length);if(count==0)break;total+=count;if(total>size)throw new Exception();hash.TransformBlock(buf,0,count,null,0);output.Write(buf,0,count);}hash.TransformFinalBlock(new byte[0],0,0);if(total!=size || !Same(before,hash.Hash) || !Same(before,Hash(f)) || !Stable(info,Info(h,false)))throw new Exception();output.Write(hash.Hash,0,32);output.Flush();}
 }
 }catch{Environment.ExitCode=1;}finally{foreach(var h in guards)h.Dispose();}}
}
