# escape=`
ARG BASE_IMAGE=mcr.microsoft.com/windows/servercore:ltsc2022
FROM ${BASE_IMAGE}
SHELL ["powershell.exe", "-NoLogo", "-NoProfile", "-Command"]
COPY OpenSSH/ C:/OpenSSH/
COPY init.ps1 receiver.ps1 start.ps1 CRTReceiver.cs C:/fixture/
RUN & C:/fixture/init.ps1
EXPOSE 22
CMD ["powershell.exe", "-NoLogo", "-NoProfile", "-File", "C:/fixture/start.ps1"]
