/*
 * COMMAND4.C:
 *
 *      Additional user routines
 *
 *      Copyright (C) 1991, 1992, 1993 Brett J. Vickers
 *
 */

#include "mstruct.h"
#include "mextern.h"
#include <sys/time.h>
#include <string.h>
#include <stdio.h>
#include "mtype.h"
/**********************************************************************/
/*                              health                                */
/**********************************************************************/

/* This function shows a player his current hit points, magic points,  */
/* experience, gold and level.                                         */

int health(ply_ptr, cmnd)
creature        *ply_ptr;
cmd             *cmnd;
{
        int     fd;
	char 	str[64];
        long    expneeded;
        unsigned long int exper_ply_1000;

        fd = ply_ptr->fd;

        if(F_ISSET(ply_ptr, PBLIND)){
                ANSI(fd, RED);
                ANSI(fd, BOLD);
                ANSI(fd, BLINK);
                print(fd, "당신은 눈이 멀어 있습니다!");
                ANSI(fd, NORMAL);
                ANSI(fd, WHITE);
                return(0);
        }
                ANSI(fd, WHITE);
        print(fd, "%s : %s (레벨 %d)", ply_ptr->name,
                title_ply(ply_ptr,ply_ptr), ply_ptr->level);
        if(F_ISSET(ply_ptr, PHIDDN)) {
                ANSI(fd, CYAN);
                print(fd, " *은신* ");
        }
        if(F_ISSET(ply_ptr, PPOISN)) {
                ANSI(fd, BLINK);
                ANSI(fd, GREEN);
                print(fd, " *중독* ");
                ANSI(fd, NORMAL);
        }
        if(F_ISSET(ply_ptr, PCHARM)) {
                ANSI(fd, BOLD);
                ANSI(fd, CYAN);
                print(fd, " *최면* ");
                ANSI(fd, NORMAL);
        }
        if(F_ISSET(ply_ptr, PSILNC)) {
                ANSI(fd, BLINK);
                ANSI(fd, MAGENTA);
                print(fd, " *벙어리* ");
                ANSI(fd, NORMAL);
        }
        if(F_ISSET(ply_ptr, PDISEA)) {
                ANSI(fd, BLINK);
                ANSI(fd, RED);
                print(fd," *질병* ");
                ANSI(fd, NORMAL);
        }
        if(ply_ptr->level < MAXALVL) 
                expneeded = needed_exp[ply_ptr->level-1];
        else
                expneeded = needed_exp[MAXALVL-2]+(ply_ptr->level-MAXALVL+1)*5000000;

/*              expneeded = (long)((needed_exp[MAXALVL-1]*ply_ptr->level));
*/
        ANSI(fd, GREEN);
	sprintf(str,"%d/%d",ply_ptr->hpcur, ply_ptr->hpmax);
        print(fd, "\n [체  력] %-16s",str);
	sprintf(str,"%d/%d",ply_ptr->mpcur, ply_ptr->mpmax);
	print(fd, " [도  력] %-16s",str);
        ANSI(fd, RED);
        print(fd, "[방어력] %d\n", (100-ply_ptr->armor));

	if (ply_ptr->class == CARETAKER || ply_ptr->class == BULSA)
	{
	/* ANSI(fd, YELLOW);
	print(fd, " [향상치] %-16lu [  돈  ] %-16ld",
		(ply_ptr->experience - 100000000), ply_ptr->gold); - 수정 */
	ANSI(fd, YELLOW);
    if(ply_ptr->experience < 100000000) exper_ply_1000 = 0;
         else exper_ply_1000 = ply_ptr->experience - 100000000;
    print(fd, " [향상치] %-16lu [  돈  ] %-16ld",
        exper_ply_1000, ply_ptr->gold);
    }
	else
	{
        ANSI(fd, YELLOW);
        print(fd, " [목표치] %-16lu [  돈  ] %-16ld",
                MAX(0,expneeded-ply_ptr->experience), ply_ptr->gold);
	}
        ANSI(fd, MAGENTA);
        print(fd, "[용  기] %d\n", (20-ply_ptr->thaco));

        ANSI(fd, CYAN);
	print(fd, "\n 당신은");
	print(fd, " %s", ansi_print(ply_ptr, ply_ptr->description));
        ANSI(fd, CYAN);
	print(fd, "있습니다.");
        ANSI(fd, WHITE);
        ANSI(fd, NORMAL);
        return(0);
}

/**********************************************************************/
/*                              help                                  */
/**********************************************************************/

/* This function allows a player to get help in general, or help for a */
/* specific command.  If help is typed by itself, a list of commands   */
/* is produced.  Otherwise, help is supplied for the command specified */

int help(ply_ptr, cmnd)
creature        *ply_ptr;
cmd             *cmnd;
{
        char    file[80];
        int     fd, c=0, match=0, num=0;
        int current_deep,match_deep=0;

        fd = ply_ptr->fd;
        strcpy(file, DOCPATH);

        if(cmnd->num < 2) {
                strcat(file, "/helpfile");
                view_file(fd, 1, file);
                return(DOPROMPT);
        }
        if(!strcmp(cmnd->str[1], "주술")) {
                strcat(file, "/spellfile");
                view_file(fd, 1, file);
                return(DOPROMPT);
        }

        if(!strcmp(cmnd->str[1], "정책")) {
                strcat(file, "/policy");
                view_file(fd, 1, file);
                return(DOPROMPT);
        }

        do {
                if(!strcmp(cmnd->str[1], cmdlist[c].cmdstr)) {
                        match = 1;
                        num = c;
                        break;
                }
		else if((current_deep=str_compare(cmnd->str[1], cmdlist[c].cmdstr))!=0) {
			match = 1;
			if(match_deep==0 || match_deep>current_deep) {
				num = c;
				match_deep = current_deep;
			}
		}
                c++;
        } while(cmdlist[c].cmdno);

        if(match == 1) {
                sprintf(file, "%s/help.%d", DOCPATH, cmdlist[num].cmdno);
                view_file(fd, 1, file);
                return(DOPROMPT);
        }
        else if(match > 1) {
                print(fd, "명령을 잘 모르겠네요.");
                return(0);
        }

        c = num = match_deep=0;
        do {
                if(!strcmp(cmnd->str[1], spllist[c].splstr)) {
                        match = 1;
                        num = c;
                        break;
                }
		else if((current_deep=str_compare(cmnd->str[1], spllist[c].splstr))!=0) {
			match = 1;
			if(match_deep==0 || match_deep>current_deep) {
				num=c;
				match_deep = current_deep;
			}
		}
                c++;
        } while(spllist[c].splno != -1);

        if(match == 0) {
                print(fd, "그 명령어에 대한 도움말은 없습니다.");
                return(0);
        }
/*
        else if(match > 1) {
                print(fd, "주술 이름을 잘모르겠네요.");
                return(0);
        }
*/

        sprintf(file, "%s/spell.%d", DOCPATH, spllist[num].splno);
        view_file(fd, 1, file);
        return(DOPROMPT);
}

/**********************************************************************/
/*                              welcome                               */
/**********************************************************************/

/* Outputs welcome file to user, giving him/her info on how to play   */
/* the game                                                           */

int welcome(ply_ptr, cmnd)
creature        *ply_ptr;
cmd             *cmnd;
{
        char    file[80];
        int     fd;

        fd = ply_ptr->fd;

        sprintf(file, "%s/welcome", DOCPATH);

        view_file(fd, 1, file);
        return(DOPROMPT);
}

/**********************************************************************/
/*                              info                                  */
/**********************************************************************/

/* This function displays a player's entire list of information, including */
/* player stats, proficiencies, level and class.                           */

int info(ply_ptr, cmnd)
creature        *ply_ptr;
cmd             *cmnd;
{
        FILE *fp;
        char alstr[16];
        char file[80];
        char str[15];
        int     fd, cnt;
        long    expneeded, lv;
   
        fd = ply_ptr->fd;
/*
        update_ply(ply_ptr);
*/
        if(ply_ptr->level < MAXALVL)
                expneeded = needed_exp[ply_ptr->level -1];
        else
                expneeded = needed_exp[MAXALVL-2]+(ply_ptr->level-MAXALVL+1)*5000000;

        if(ply_ptr->alignment < -100)
                strcpy(alstr, " (악합니다)");
        else if(ply_ptr->alignment < 101)
                strcpy(alstr, " (평범합니다)");
        else
                strcpy(alstr, " (선합니다) ");

        for(lv=0,cnt=0; lv<MAXWEAR; lv++)
                if(ply_ptr->ready[lv]) cnt++;
        cnt += count_inv(ply_ptr, -1);

        if(!F_ISSET(ply_ptr , PMARRI)) strcpy(str ,"없음");
        else {   
	   sprintf(file, "%s/marriage/%s", PLAYERPATH, ply_ptr->name);
	   fp = fopen(file, "r");
	   fscanf(fp, "%s", str);
           fclose(fp);
	}
	   
        print(fd, "\n[이  름] %s        [배우자] %s\n", ply_ptr->name, str);
        print(fd, "[칭  호] %s\n\n", title_ply(ply_ptr,ply_ptr));
        print(fd, "[레  벨] %-20d       [종  족] %s\n",
                ply_ptr->level, race_str[ply_ptr->race]);
        print(fd, "[직  업] %-20s       [성  향] %s %s\n",
                class_str[ply_ptr->class],
                F_ISSET(ply_ptr, PCHAOS) ? "악":"선", alstr);

#define INTERVAL ply_ptr->lasttime[LT_HOURS].interval
        print(fd, "접속시간 : ");
        if(INTERVAL > 86400L)
                print(fd, "%d일 ", INTERVAL/86400L);
        if(INTERVAL > 3600L)
                print(fd, "%d시간 ", (INTERVAL % 86400L)/3600L);
        print(fd, "%d분\n\n", (INTERVAL % 3600L)/60L);

        print(fd, "[  힘  ] %-2d      [민  첩] %-2d      [맷  집] %-2d\n",
              ply_ptr->strength, ply_ptr->dexterity, ply_ptr->constitution);
        print(fd, "[지  식] %-2d      [신앙심] %-2d      [용  기] %-2d\n\n",
              ply_ptr->intelligence, ply_ptr->piety, (20-ply_ptr->thaco));
        print(fd,
         "[체  력] %-5d/%-5d          [경험치] %lu ( %lu의 경험치 필요)\n",
         ply_ptr->hpcur, ply_ptr->hpmax, ply_ptr->experience,
         MAX(0, expneeded-ply_ptr->experience));
        print(fd, "[도  력] %-5d/%-5d          [  돈  ] %-7lu\n",
              ply_ptr->mpcur, ply_ptr->mpmax, ply_ptr->gold);
        print(fd, "[방어력] %-5d                [소지품 무게] %d 근 (총 %d개).\n\n",
              (100-ply_ptr->armor), weight_ply(ply_ptr), cnt);

        ANSI(fd, GREEN);
        print(fd, "## 무기사용능력 ##\n");
        ANSI(fd, WHITE);
        ANSI(fd, NORMAL);
        print(fd, "\n[ 도 ] %2d%%         [ 검 ] %2d%%         [ 봉 ] %2d%%\n",
              profic(ply_ptr, SHARP), profic(ply_ptr, THRUST),
              profic(ply_ptr, BLUNT));
        print(fd, "[ 창 ] %2d%%         [ 궁 ] %2d%%\n\n",
              profic(ply_ptr, POLE), profic(ply_ptr, MISSILE));

        F_SET(ply_ptr, PREADI);
        print(fd, "[엔터]를 누르세요. 그만보시려면 [.]을 치세요: ");
        output_buf();
        Ply[fd].io->intrpt &= ~1;
        Ply[fd].io->fn = info_2;
        Ply[fd].io->fnparam = 1;
        return(DOPROMPT);
}

/************************************************************************/
/*                              info_2                                  */
/************************************************************************/

/* This function is the second half of info which outputs spells        */

void info_2(fd, param, instr)
int     fd, param;
char    *instr;
{
        char            str[2048];
        char            spl[128][20];
        int             i, j, k=0, l;
        creature        *ply_ptr;

        ply_ptr = Ply[fd].ply;

        if(instr[0]=='.') {
                print(fd, "중단되었습니다.\n");
                F_CLR(ply_ptr, PREADI);
                RETURN(fd, command, 1);
        }

        ANSI(fd, GREEN);
        print(fd, "\n## 주 술  계 열 ##\n\n");
        ANSI(fd, WHITE);
        ANSI(fd, NORMAL);
        print(fd,
           "[ 땅 ] %2d%%      [바람] %2d%%    [ 불 ] %2d%%   [ 물 ] %2d%%\n\n",
           mprofic(ply_ptr, EARTH), mprofic(ply_ptr, WIND),
           mprofic(ply_ptr, FIRE), mprofic(ply_ptr, WATER));
   
        strcpy(str, "\n주문: ");
        for(i=0,j=0; i<66; i++)
                if(S_ISSET(ply_ptr, i))
                        strcpy(spl[j++], spllist[i].splstr);

        if(!j)
                strcat(str, "없음.");
        else {
                qsort((void *)spl, j, 20, strcmp);
                for(i=0; i<j; i++) {
                        strcat(str, spl[i]);
                        strcat(str, ", ");
                }
                str[strlen(str)-2] = '.';
                str[strlen(str)-1] = 0;
        }
        print(fd, "%s\n", str);

        strcpy(str, "당신의 현주문: ");
        if(F_ISSET(ply_ptr, PBLESS)) strcat(str, "성현진, ");
        if(F_ISSET(ply_ptr, PLIGHT)) strcat(str, "발광, ");
        if(F_ISSET(ply_ptr, PPROTE)) strcat(str, "수호진, ");
        if(F_ISSET(ply_ptr, PINVIS)) strcat(str, "은둔법, ");
        if(F_ISSET(ply_ptr, PDINVI)) strcat(str, "은둔감지, ");
        if(F_ISSET(ply_ptr, PDMAGI)) strcat(str, "주문감지, ");
        if(F_ISSET(ply_ptr, PLEVIT)) strcat(str, "부양술, ");
        if(F_ISSET(ply_ptr, PRFIRE)) strcat(str, "방열진, ");
        if(F_ISSET(ply_ptr, PFLYSP)) strcat(str, "비상술, ");
        if(F_ISSET(ply_ptr, PRMAGI)) strcat(str, "보마진, ");
        if(F_ISSET(ply_ptr, PKNOWA)) strcat(str, "선악감지, ");
        if(F_ISSET(ply_ptr, PRCOLD)) strcat(str, "방한진, ");
        if(F_ISSET(ply_ptr, PBRWAT)) strcat(str, "수생술, ");
        if(F_ISSET(ply_ptr, PSSHLD)) strcat(str, "지방호, ");
        if(strlen(str) == 15)
                strcat(str, "없음.");
        else {
                str[strlen(str)-2] = '.';
                str[strlen(str)-1] = 0;
        }
        print(fd, "%s\n", str);

        i=0;
        while(Q_ISSET(ply_ptr,i)) i++;
        if(i==0) print(fd,"당신은 현재 달성한 임무가 없습니다.");
        else 
          {
           print(fd,"당신은 현재 ");
           
           ANSI(fd, MAGENTA);
           print(fd, "임무 %d", i);
           ANSI(fd, WHITE);
           ANSI(fd, NORMAL);
           print(fd, "까지 달성하였습니다.");
          }
        if(ply_ptr->class >= INVINCIBLE) {
            print(fd, "\n무적수련 : ");
            for(l=0 ; l<8 ; l++) {
	                    if(S_ISSET(ply_ptr, l+SASSASSIN)) {
			                               print(fd, class_str[l+1]);
			                               print(fd, " ");
			                               k++;
			    }
	    }
         if(k<1) print(fd, "없음\n\n");
	 else  print(fd, "\n\n");  
	}
   
        F_CLR(Ply[fd].ply, PREADI);
        RETURN(fd, command, 1);
}

/**********************************************************************/
/*                              send                                  */
/**********************************************************************/

/* This function allows a player to send a message to another player.  If */
/* the other player is logged in, the message is sent successfully.       */

int send(ply_ptr, cmnd)
creature        *ply_ptr;
cmd             *cmnd;
{
        creature        *crt_ptr = 0;
        etag            *ign;
        int             spaces=0, i, fd;
        int             len;

        fd = ply_ptr->fd;

        if(cmnd->num < 2) {
                print(fd, "누구에게 말을 전하시려구요?");
                return 0;
        }

        cmnd->str[1][0] = up(cmnd->str[1][0]);
        for(i=0; i<Tablesize; i++) {
                if(!Ply[i].ply) continue;
                if(Ply[i].ply->fd == -1) continue;
                if(F_ISSET(Ply[i].ply, PDMINV) && ply_ptr->class < DM)
                        continue;
                if(!strncmp(Ply[i].ply->name, cmnd->str[1],
                   strlen(cmnd->str[1])))
                        crt_ptr = Ply[i].ply;
                if(!strcmp(Ply[i].ply->name, cmnd->str[1]))
                        break;
        }
        if(!crt_ptr) {
                print(fd, "누구에게 말을 전하시려구요?");
                return(0);
        }
        if(ply_ptr->class < DM && (F_ISSET(crt_ptr, PINVIS) && !F_ISSET(ply_ptr, PDINVI))) {
                print(fd, "누구에게 말을 전하시려구요?");
                return(0);
        }
        if(F_ISSET(crt_ptr, PIGNOR) && (ply_ptr->class < DM )) {
                print(fd, "%s님은 이야기 듣기 거부 상태입니다.", crt_ptr->name);
                return(0);
        }

        ign = Ply[crt_ptr->fd].extr->first_ignore;
        while(ign) {
                if(!strcmp(ign->enemy, ply_ptr->name)) {
                        print(fd, "%s is ignoring you.", crt_ptr->name);
                        return(0);
                }
                ign = ign->next_tag;
        }
        
        len = cut_command(cmnd->fullstr);
        for(i=0; i< len && i<256; i++) {
                if(cmnd->fullstr[i] == ' ' && cmnd->fullstr[i+1] != ' ') {
                        spaces++;
                        break;
                }
        }
        cmnd->fullstr[255] = 0;

        if(spaces < 1 || strlen(&cmnd->fullstr[i+1]) < 1) {
                print(fd, "무슨 말을 전하시려구요?");
                return(0);
        }
        if(F_ISSET(ply_ptr, PSILNC)) {
                print(fd, "당신은 말을 할 수 없습니다.");
                return(0);
        }
        if(F_ISSET(ply_ptr, PLECHO)){
                print(fd, "당신은 %M에게 \"%s\"라고 이야기합니다.", crt_ptr,&cmnd->fullstr[i+1]);
        }
        else
                print(fd, "%s님에게 말을 전달하였습니다.", crt_ptr->name);
        
        print(crt_ptr->fd, "\n%C%M%j 당신에게 \"%s\"라고 이야기합니다.%D", "31",ply_ptr,"1",
              &cmnd->fullstr[i+1],"37");

        strcpy(Ply[crt_ptr->fd].extr->talksend, ply_ptr->name);

        if(ply_ptr->class <= BULSA && crt_ptr->class <= BULSA) {
        	broadcast_eaves("\n--- %s님이 %s님에게 \"%s\"라고 이야기합니다.", ply_ptr->name, crt_ptr->name, &cmnd->fullstr[i+1]);
	}

        paste_command(cmnd->fullstr,len);
        return(0);

}

/**********************************************************************/
/*                              broadsend                             */
/**********************************************************************/

/* This function is used by players to broadcast a message to every   */
/* player in the game.  Broadcasts by players are of course limited,  */
/* so the number used that day is checked before the broadcast is     */
/* allowed.                                                           */
long broad_time[PMAX];
long all_broad_time=0;
char broad_str[PMAX][256]; /* 전에 한말을 저장 */
long broad_limit_time[PMAX]; /* 잡담 제한 시간 */

int broadsend(ply_ptr, cmnd)
creature        *ply_ptr;
cmd             *cmnd;
{
        int     i, found=0, fd;
        int     len;
        int discount=2;
        long    t,tt;
	char     str[1024];
	
        int dc_table[31]={
           60, 57, 54, 51, 48, 45, 42, 40, 38, 36,
           34, 32, 30, 28, 26, 24, 22, 20, 18, 16,
           14, 12, 10,  8,  7,  6,  5,  4,  3,  2,  2, };

        fd = ply_ptr->fd;
        len = cut_command(cmnd->fullstr);
        cmnd->fullstr[255] = 0;

        if(len==0) {
                print(fd, "무슨 말을 하시려구요?");
                return(0);
        }

        tt=time(0);
        t=tt-all_broad_time;

/*
        if(t>10 && ply_ptr->class<CARETAKER) {
            if(!dec_daily(&ply_ptr->daily[DL_BROAD])) {
                print(fd,"당신은 오늘 잡담의 한계를 넘겼습니다.");
                return(0);
            }
        }
*/
      
	if(F_ISSET(ply_ptr, PSILNC) && ply_ptr->class<SUB_DM) {
                print(fd, "당신의 목소리가 너무 작아 잡담을 할 수 없습니다.");
                return(0);
        }
        
        if(ply_ptr->level < 20 && ply_ptr->class < CARETAKER) {
        	print(fd, "당신의 레벨로는 잡담을 할 수 없습니다.");
        	return(0);
        }
	
	if (strcmp(cmnd->fullstr, broad_str[ply_ptr->fd]) == 0) {
	    	if ((broad_limit_time[ply_ptr->fd] - time(0)) > 0) {
		    ANSI(ply_ptr->fd, CYAN);
		    print(ply_ptr->fd, "\n도배하지 마세요.\n");
        	    ANSI(ply_ptr->fd, WHITE);
		    ANSI(ply_ptr->fd, NORMAL);
		    return (0);
		}
	}

		    
	t=tt-broad_time[fd];

        if(t<0 || t>30) discount=2;
        else discount=dc_table[t];

        if(ply_ptr->level<10) discount/=2;
        if(ply_ptr->level<20) discount/=2;

        if(ply_ptr->hpcur<=discount && ply_ptr->class<SUB_DM) {
            print(fd,"당신의 목숨이 위태로워 잡담을 할 수 없습니다.");
            return 0;
        }
        if(ply_ptr->class<SUB_DM) ply_ptr->hpcur-=discount;
        broad_time[fd]=tt;
	/*
        broadcast("\n%C%s> %s%D", "32",ply_ptr->name, &cmnd->fullstr[0],"37");
	*/
	strcpy(broad_str[ply_ptr->fd], cmnd->fullstr);
	sprintf(str ,"\n{녹%s> %s}", ply_ptr->name, &cmnd->fullstr[0]);  
        broadcast_ansi(str);
	broad_limit_time[ply_ptr->fd] = time(0) + 5L; /* 잡담 제한 타임 설정 */
        return(0);

}

char broad_str2[PMAX][256];
long broad_limit_time2[PMAX];

int broadsend2(ply_ptr, cmnd)
creature        *ply_ptr;
cmd             *cmnd;
{
        int     i, found=0, fd;
        int     len;
        int discount=2;
        long t,tt;
        int dc_table[31]={
           60, 57, 54, 51, 48, 45, 42, 40, 38, 36,
           34, 32, 30, 28, 26, 24, 22, 20, 18, 16,
           14, 12, 10,  8,  7,  6,  5,  4,  3,  2,  2, };

        fd = ply_ptr->fd;
        len = cut_command(cmnd->fullstr);
        cmnd->fullstr[255] = 0;

        if(len==0) {
                print(fd, "무슨 말을 하시려구요?");
                return(0);
        }

	if (strcmp(broad_str2[ply_ptr->fd], cmnd->fullstr) == 0) {
	    	if (broad_limit_time2[ply_ptr->fd] - time(0) > 0) {
		    	ANSI(ply_ptr->fd, CYAN);
			print(ply_ptr->fd, "\n도배하지 마세요\n");
			ANSI(ply_ptr->fd, WHITE);
			ANSI(ply_ptr->fd, NORMAL);
			return (0);
		}
	}
	
        tt=time(0);
        t=tt-all_broad_time;

/*
        if(t>10 && ply_ptr->class<CARETAKER) {
            if(!dec_daily(&ply_ptr->daily[DL_BROAD])) {
                print(fd,"당신은 오늘 잡담의 한계를 넘겼습니다.");
                return(0);
            }
        }
*/
      
	if(F_ISSET(ply_ptr, PSILNC) && ply_ptr->class<SUB_DM) {
                print(fd, "당신의 목소리가 너무 작아 환호를 할 수 없습니다.");
                return(0);
        }
        
        if(ply_ptr->level < 20 && ply_ptr->class < CARETAKER) {
        	print(fd, "당신의 레벨로는 환호를 할 수 없습니다.");
        	return(0);
        }

        t=tt-broad_time[fd];

        if(t<0 || t>30) discount=2;
        else discount=dc_table[t];

        if(ply_ptr->level<10) discount/=2;
        if(ply_ptr->level<20) discount/=2;

        if(ply_ptr->hpcur<=discount && ply_ptr->class<SUB_DM) {
            print(fd,"당신의 목숨이 위태로워 환호를 할 수 없습니다.");
            return 0;
        }
        if(ply_ptr->class<SUB_DM) ply_ptr->hpcur-=discount;
        broad_time[fd]=tt;
	strcpy(broad_str2[ply_ptr->fd], cmnd->fullstr);
	
        broadcast2("\n%C%s님이 \"%s\"라고 환호를 합니다.%D", "32",ply_ptr->name, &cmnd->fullstr[0],"37");
	broad_limit_time2[ply_ptr->fd] = time(0) + 5L;
        return(0);
}

/**********************************************************************/
/*                              follow                                */
/**********************************************************************/

/* This command allows a player (or a monster) to follow another player. */
/* Follow loops are not allowed; i.e. you cannot follow someone who is   */
/* following you.  Also, you cannot follow yourself.                     */

int follow(ply_ptr, cmnd)
creature        *ply_ptr;
cmd             *cmnd;
{
  creature        *old_ptr, *new_ptr;
  room            *rom_ptr;
  ctag            *cp, *pp, *prev;
  int             fd;
  int 		  fol_cnt = 0;
  ctag		  *cp2;
  fd = ply_ptr->fd;
  rom_ptr = ply_ptr->parent_rom;
  
  /* 잠시 사용중지 */
  /*      print(fd,"버그가 있는관계로 사용을 금지합니다.");
	  return;
	  */
  if(cmnd->num < 2) {
    print(fd, "누구를 따라 가시고 싶으세요?");
    return(0);
  }
  
  F_CLR(ply_ptr, PHIDDN);
  if(!strcmp(cmnd->str[1],"나")) new_ptr=ply_ptr;
  else new_ptr = find_crt(ply_ptr, rom_ptr->first_ply,
			  cmnd->str[1], cmnd->val[1]);
  
  if(!new_ptr) {
    print(fd, "그런 사람은 여기 없습니다.");
    return(0);
  }
  
  if(new_ptr == ply_ptr && !ply_ptr->following) {
    print(fd, "자기자신을 따라 갈순 없습니다.");
    return(0);
  }
  
  if(new_ptr->following == ply_ptr) {
    print(fd, "이미 %s는 당신을 따라다니고 있습니다.",
	  F_ISSET(new_ptr, PMALES) ? "그":"그녀");
    return(0);
  }
  
  /* 그룹원 수 제한 */
  cp2 = new_ptr->first_fol;
  while (cp2) {
      	fol_cnt++;
	cp2 = cp2->next_tag;
  }
  if (fol_cnt > 3) {
      	print(fd, "이미 그룹원이 5명이 다 찼군요.\n");
	return (0);
  }
  
  if(ply_ptr->following) { /* 따라가고 있는 사람이 있다면... */
    old_ptr = ply_ptr->following;
    cp = old_ptr->first_fol;
    if(cp->crt == ply_ptr) {
      old_ptr->first_fol = cp->next_tag;
      free(cp);
    }
    else while(cp) {
      if(cp->crt == ply_ptr) {
	prev->next_tag = cp->next_tag;
	free(cp);
	break;
      }
      prev = cp;
      cp = cp->next_tag;
    }
    ply_ptr->following = 0;
    print(fd, "당신은 %s님을 그만 따라 다니기로 하였습니다.\n", old_ptr->name);
    if(!F_ISSET(ply_ptr, PDMINV))
      print(old_ptr->fd, "\n%M은 이제 당신을 따라다니지 않습니다.", 
	    ply_ptr);
  }
  
  if(ply_ptr == new_ptr)
    return(0);
  
  ply_ptr->following = new_ptr;
  
  pp = (ctag *)malloc(sizeof(ctag));
  if(!pp)
    merror("follow", FATAL);
  pp->crt = ply_ptr;
  pp->next_tag = 0;
  
  if(!new_ptr->first_fol)
    new_ptr->first_fol = pp;
  else {
    pp->next_tag = new_ptr->first_fol;
    new_ptr->first_fol = pp;
  }
  
  print(fd, "당신은 이제부터 %s님을 따라다닙니다.", new_ptr->name);
  if(!F_ISSET(ply_ptr, PDMINV)) {
    print(new_ptr->fd, "\n%M이 이제부터 당신을 따라다닙니다.", ply_ptr);
    broadcast_rom2(fd, new_ptr->fd, ply_ptr->rom_num,
		   "\n%M이 이제부터 %M을 따라다닙니다.", ply_ptr, new_ptr);
  }
  
  return(0);
  
}

/**********************************************************************/
/*                              lose                                  */
/**********************************************************************/

/* This function allows a player to lose another player who might be  */
/* following him.  When successful, that player will no longer be     */
/* following.                                                         */

int lose(ply_ptr, cmnd)
creature        *ply_ptr;
cmd             *cmnd;
{
  creature        *crt_ptr;
  ctag            *cp, *prev;
  int             fd;
  
  fd = ply_ptr->fd;
  
  if(cmnd->num == 1) {
    
    if (ply_ptr->following == 0){
      print(fd, "당신은 누구를 따라다니고 있지 않습니다.");
      return(0);
      
    }
    crt_ptr = ply_ptr->following;
    cp = crt_ptr->first_fol;
    if(cp->crt == ply_ptr) {
      crt_ptr->first_fol = cp->next_tag;
      free(cp);
    }
    else while(cp) {
      if(cp->crt == ply_ptr) {
	prev->next_tag = cp->next_tag;
	free(cp);
	break;
      }
      prev = cp;
      cp = cp->next_tag;
    }
    ply_ptr->following = 0; 
    print(fd,"당신은 %M을 그만 따라다니기로 하였습니다.\n",crt_ptr);
    if (!F_ISSET(ply_ptr,PDMINV))
      print(crt_ptr->fd,"\n%M은 이제 당신을 따라 다니지 않습니다.",ply_ptr);
    return(0);
  }                   
  
  F_CLR(ply_ptr, PHIDDN);
  
  lowercize(cmnd->str[1], 1);
  crt_ptr = find_crt(ply_ptr, ply_ptr->first_fol,
		     cmnd->str[1], cmnd->val[1]);
  
  if(!crt_ptr) {
    print(fd, "그 사람은 당신을 따라다니고 있지 않습니다.");
    return(0);
  }
  
  if(crt_ptr->following != ply_ptr) {
    print(fd, "그 사람은 당신을 따라다니고 있지 않습니다.");
    return(0);
  }
  
  cp = ply_ptr->first_fol;
  if(cp->crt == crt_ptr) {
    ply_ptr->first_fol = cp->next_tag;
    free(cp);
  }
  else while(cp) {
    if(cp->crt == crt_ptr) {
      prev->next_tag = cp->next_tag;
      free(cp);
      break;
    }
    prev = cp;
    cp = cp->next_tag;
  }
  crt_ptr->following = 0;
  
  print(fd, "당신은 %M이 당신을 못따라 오도록 하였습니다.", crt_ptr);
  if(!F_ISSET(ply_ptr, PDMINV)) {
    print(crt_ptr->fd, "\n%M이 당신이 못따라 오도록 하였습니다.", ply_ptr);
    broadcast_rom2(fd, crt_ptr->fd, "\n%M이 %M을 못따라 오도록 하였습니다.", 
		   ply_ptr, crt_ptr);
  }
  
  return(0);
  
}

/**********************************************************************/
/*                              group                                 */
/**********************************************************************/

/* This function allows you to see who is in a group or party of people */
/* who are following you.                                               */

int group(ply_ptr, cmnd)
creature        *ply_ptr;
cmd             *cmnd;
{
  ctag    *cp;
  char    str[2048];
  char    str2[256];
  int     fd, found = 0;
  creature *leader;
  
  fd = ply_ptr->fd;
  
  /* 잠시 사용중지 */
  /*      print(fd,"버그가 있는 관계로 사용을 금지합니다.");
	  return;
	  */
  if(ply_ptr->following) {
    cp = ply_ptr->following->first_fol;
    leader=ply_ptr->following;
  }
  else {
    cp = ply_ptr->first_fol;
    leader=ply_ptr;
    if(!cp) {
      print(fd,"당신은 그룹에 속해 있지 않습니다.");
      return(0);
    }
  }
  
  strcpy(str, "그룹원:\n");
  sprintf(str2, "  %14s  체력:%4d/%4d 도력:%4d/%4d (대장)\n",
	  crt_str(leader,0,2),leader->hpcur,leader->hpmax,
leader->mpcur, leader->mpmax);
  strcat(str,str2);
  
  while(cp) {
    if(!F_ISSET(cp->crt, PDMINV)) {
      sprintf(str2, "  %14s  체력:%4d/%4d 도력:%4d/%4d\n",
crt_str(cp->crt,0,2),cp->crt->hpcur,cp->crt->hpmax,cp->crt->mpcur,cp->crt->mpmax);
      strcat(str, str2);
      found = 1;
    }
    cp = cp->next_tag;
  }
  
  if(!found) {
    print(fd, "당신은 그룹에 속해 있지 않습니다.");
    return(0);
  }
  
  str[strlen(str)-1] = 0;
  print(fd, "%s", str);
  
  return(0);
  
}

/**********************************************************************/
/*                              track                                 */
/**********************************************************************/

/* This function is the routine that allows rangers to search for tracks */
/* in a room.  If the ranger is successful, he will be told what dir-    */
/* ection the last person who was in the room left.                      */

int track(ply_ptr, cmnd)
creature        *ply_ptr;
cmd             *cmnd;
{
        long    i, t;
        int     fd, chance;

        fd = ply_ptr->fd;

        if(ply_ptr->class != RANGER && ply_ptr->class < INVINCIBLE) {
                print(fd, "포졸만 쓸수 있는 명령입니다.");
                return(0);
        }

        F_CLR(ply_ptr, PHIDDN);

        t = time(0);
        i = LT(ply_ptr, LT_TRACK);

        if(t < i) {
                please_wait(fd, i-t);
                return(0);
        }

        ply_ptr->lasttime[LT_TRACK].ltime = t;
        ply_ptr->lasttime[LT_TRACK].interval = 5 - bonus[ply_ptr->dexterity];
        
        if(F_ISSET(ply_ptr, PBLIND)) {
                print(fd, "당신은 눈이 멀어 있습니다. 도저히 추적을 할 수 없습니다.");
                return(0);
        }
        chance = 25 + (bonus[ply_ptr->dexterity] + ((ply_ptr->level+3)/4))*5;

        if(mrand(1,100) > chance) {
                print(fd, "추적 실패!");
                return(0);
        }

        if(!ply_ptr->parent_rom->track[0]) {
                print(fd, "아무런 흔적이 남아있지 않습니다.");
                return(0);
        }

        print(fd, "%s쪽으로 흔적이 나 있습니다.",
              ply_ptr->parent_rom->track);
        broadcast_rom(fd, ply_ptr->rom_num, "\n%M이 적이 지나간 흔적을 찾았습니다.", ply_ptr);

        return(0);

}

/**********************************************************************/
/*                              peek                                  */
/**********************************************************************/

/* This function allows a thief or assassin to peek at the inventory of */
/* another player.  If successful, they will be able to see it and      */
/* another roll is made to see if they get caught.                      */

int peek(ply_ptr, cmnd)
creature        *ply_ptr;
cmd             *cmnd;
{
        creature        *crt_ptr;
        room            *rom_ptr;
        char            str[4096];
        long            i, t;
        int             fd, n, chance;

        fd = ply_ptr->fd;
        rom_ptr = ply_ptr->parent_rom;
        str[0] = 0;

        if(cmnd->num < 2) {
                print(fd, "누구의 소지품을 보려구요?");
                return(0);
        }

        if(ply_ptr->class != THIEF && ply_ptr->class < INVINCIBLE) {
                print(fd, "당신 직업으로는 다른사람의 소지품을 볼 수 없습니다.");
                return(0);
        }

        if(F_ISSET(ply_ptr, PBLIND)){
                ANSI(fd, RED);
                print(fd, "당신은 눈이 멀어 있습니다!");
                ANSI(fd, WHITE);
                return(0);
        }

        crt_ptr = find_crt(ply_ptr, rom_ptr->first_mon,
                           cmnd->str[1], cmnd->val[1]);
        if(!crt_ptr) {
                lowercize(cmnd->str[1], 1);
                crt_ptr = find_crt(ply_ptr, rom_ptr->first_ply,

                                   cmnd->str[1], cmnd->val[1]);

                if(!crt_ptr) {
                        print(fd, "그런 사람 없어요!");
                        return(0);
                }
        }

        i = LT(ply_ptr, LT_PEEKS);
        t = time(0);

        if(i > t) {
                please_wait(fd, i-t);
                return(0);
        }

        ply_ptr->lasttime[LT_PEEKS].ltime = t;
        ply_ptr->lasttime[LT_PEEKS].interval = 5;

        if((F_ISSET(crt_ptr, MUNSTL) || F_ISSET(crt_ptr, MTRADE) || F_ISSET(crt_ptr, MPURIT)) && ply_ptr->class < DM) {
                print(fd, "당신은 다른사람의 소지품을 볼 수 없습니다.\n다른사람이 당신보고 도둑이라고 생각할 것입니다.");
                return(0);
        }

        chance = (25 + ((ply_ptr->level+3)/4)*10)-(((crt_ptr->level+3)/4)*5);
        if (chance<0) chance=0;
        if (ply_ptr->class >= BULSA) chance=100;
	else if (ply_ptr->class == CARETAKER) chance=90;
	
        if(mrand(1,100) > chance) {
                print(fd, "실패하였습니다!");
                return(0);
        }

        chance = MIN(90, 15 + ((ply_ptr->level+3)/4)*5);

        if(mrand(1,100) > chance && ply_ptr->class < CARETAKER) {
                print(crt_ptr->fd, "%s님이 당신의 소지품을 슬쩍 엿봅니다.", ply_ptr);
                broadcast_rom2(fd, crt_ptr->fd, ply_ptr->rom_num,
                               "\n%M이 %M의 소지품을 슬쩍 엿봅니다.",
                               ply_ptr, crt_ptr);
        }

        sprintf(str, "%s의 소지품: ", F_ISSET(crt_ptr, PMALES) ? "그":"그녀");
        n = strlen(str);
        if(list_obj(&str[n], ply_ptr, crt_ptr->first_obj) > 0)
                print(fd, "%s", str);
        else
                print(fd, "%s는 아무것도 들고 있지 않습니다.",
                      F_ISSET(crt_ptr, PMALES) ? "그":"그녀");

        return(0);

}
